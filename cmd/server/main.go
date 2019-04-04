package main

import (
	"log"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"net/http"
	"math/rand"
	"time"

	//"github.com/aliok/secim-batch-plain-operator/pkg/secimOperator"
	"flag"
	"path/filepath"
	"os/user"
	"math"
	"strconv"
	"io/ioutil"
	"encoding/json"
	"fmt"
)

const PARALLELISM = 3
const BATCH_SIZE = 5
const WORK_ITEM_COUNT = 50

const JOB_ID = "foo"
const NAMESPACE = "myproject"

const POD_POLLING_INTERVAL = 1 * time.Second

// TODO: improvements
// - env var for constants above
// - env var for in-cluster out-of-cluster config
// - annotations or labels for job pods

var finalResult map[string]interface{}
var finalErrors []string

func main() {
	finalResult = make(map[string]interface{})
	finalErrors = make([]string, 0)

	rand.Seed(time.Now().Unix())

	// TODO: in-cluster config
	// config, err := rest.InClusterConfig()
	// if err != nil {
	// 	panic(err.Error())
	// }

	// TODO: out-of-cluster config
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	home := usr.HomeDir

	var kubeconfig *string
	if home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	k8client := kubernetes.NewForConfigOrDie(config)

	//pods, err := k8client.CoreV1().Pods("myproject").List(metav1.ListOptions{})
	//if err != nil {
	//	panic(err.Error())
	//}
	//fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	queue := createWorkQueue()

	createInitialPods(queue, k8client)

	startPollingPodEndpoints(queue, k8client)
	// go startWatchingPods()

	// select {}

	// watch for 2 things:
	// - 1. pod resources: if there is one failed, get the info of that one and add it to the queue
	// - 2. pod states: whenever a pod's "status" endpoint says "DONE", get the data from it and kill it
	//      - then get the next item from the queue and create a pod for that

	//for _, podReq := range queue {
	//	var resp = &apiv1.Pod{}
	//
	//	if resp, err = k8client.CoreV1().Pods(NAMESPACE).Create(podReq); err != nil {
	//		log.Fatal(err)
	//		panic(err.Error())
	//	}
	//
	//	fmt.Printf("Pod created: %s\n", resp)
	//
	//	break
	//}

	//{
	//	pods, err := k8client.CoreV1().Pods("myproject").List(metav1.ListOptions{})
	//	if err != nil {
	//		panic(err.Error())
	//	}
	//	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))
	//}
}

//func startWatchingPods() {
//	log.Printf("Starting pod resource watching")
//
//	// TODO
//}

// poll pod list, then check if the pods are done.
// if done, record the output and the errors. then pop one from the queue and create a new pod.
// if not done, don't do anything. it will be checked in the next poll.
func startPollingPodEndpoints(queue chan *apiv1.Pod, k8client *kubernetes.Clientset) {
	log.Printf("Starting pod endpoint polling")

	for {
		<-time.After(POD_POLLING_INTERVAL)
		pods, err := k8client.CoreV1().Pods(NAMESPACE).List(metav1.ListOptions{})
		if err != nil {
			log.Fatalf("Unable to read pod list")
			log.Fatal(err)
			panic(err.Error())
		}
		log.Printf("There are %d pods in the cluster\n", len(pods.Items))

		for _, pod := range pods.Items {
			// log.Printf("Pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
			switch pod.Status.Phase {
			case apiv1.PodPending:
				// do nothing yet
			case apiv1.PodUnknown:
				// do nothing and hope that it will be known in later poll
			case apiv1.PodFailed:
				log.Printf("Pod seems failed. Going to add it to queue again. %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
				// TODO: delete the pod first
				queue <- &pod // TODO: not sure if this would work . probably not as the UID and the name can't be duplicate
			case apiv1.PodSucceeded:
				// do nothing as the pod is probably terminated by this program
			case apiv1.PodRunning:
				// check if the pod says it is done
				if isPodDone(pod) {
					log.Printf("Pod     done %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
					recordPodOutput(pod)
					log.Printf("   Output recorded, deleting pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
					deletePod(k8client, pod)
				} else {
					log.Printf("Pod NOT done %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
				}
			}
		}

		if

		//log.Println("Current result")
		//currentResultJson, err := json.MarshalIndent(finalResult, "", "  ")
		//if err != nil {
		//	fmt.Println("error marshalling current result:", err)
		//}
		//log.Println(string(currentResultJson))
		//
		//log.Println("Current errors")
		//log.Println(finalErrors)
	}
}
func deletePod(k8client *kubernetes.Clientset, pod apiv1.Pod) {
	err := k8client.CoreV1().Pods(NAMESPACE).Delete(pod.Name, metav1.NewDeleteOptions(0))
	if err != nil {
		log.Fatalf("Unable to delete pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		log.Fatal(err)
	}
}
func recordPodOutput(pod apiv1.Pod) {
	result := getResultFromPod(pod)
	errors := getErrorsFromPod(pod)

	var resultJson map[string]interface{}
	json.Unmarshal([]byte(result), &resultJson)

	var errorsJson []string
	json.Unmarshal([]byte(errors), &errorsJson)

	for k, v := range resultJson {
		finalResult[k] = v
	}

	for _, err := range errorsJson {
		finalErrors = append(finalErrors, err)
	}
}

func getResultFromPod(pod apiv1.Pod) []byte {
	resp, err := http.Get("http://" + pod.Status.PodIP + ":3000/result")
	if err != nil {
		log.Printf("Unable to read result of the Pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		log.Print(err)
		panic(err) // TODO: not the best way
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return body
}

func getErrorsFromPod(pod apiv1.Pod) []byte {
	resp, err := http.Get("http://" + pod.Status.PodIP + ":3000/errors")
	if err != nil {
		log.Printf("Unable to read errors of the Pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		log.Print(err)
		panic(err) // TODO: not the best way
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return body
}

func isPodDone(pod apiv1.Pod) bool {
	resp, err := http.Get("http://" + pod.Status.PodIP + ":3000/status")
	if err != nil {
		log.Printf("Unable to read status of the Pod %s %s %s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		log.Print(err)
		return false
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if string(body) == "DONE" {
		return true
	}
	return false
}

func createInitialPods(queue chan *apiv1.Pod, k8client *kubernetes.Clientset) {
	for i := 0; i < PARALLELISM; i++ {
		podRequest := <-queue

		// not interested in the response. we're polling them anyway
		if _, err := createPod(podRequest, k8client); err != nil {
			log.Printf("Unable to create pod \n")
			log.Fatal(err)
		}
	}
}

func createPod(podReq *apiv1.Pod, k8client *kubernetes.Clientset) (*apiv1.Pod, error) {
	return k8client.CoreV1().Pods(NAMESPACE).Create(podReq);
}

func createWorkQueue() chan *apiv1.Pod {
	chunkCount := int(math.Ceil(float64(WORK_ITEM_COUNT) / float64(BATCH_SIZE)))
	queue := make(chan *apiv1.Pod, chunkCount)

	for i := 0; i < chunkCount; i++ {
		podReq := &apiv1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "secim-batch-contaner-" + JOB_ID + "-" + strconv.Itoa(i) + "-",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Name:            "secim-batch-container",
						Image:           "aliok/secim-batch-container",
						ImagePullPolicy: apiv1.PullIfNotPresent,
						Env: []apiv1.EnvVar{
							{
								Name:  "BATCH_SIZE",
								Value: strconv.Itoa(BATCH_SIZE),
							},
							{
								Name:  "BATCH_INDEX",
								Value: strconv.Itoa(i),
							},
						},
						Ports: []apiv1.ContainerPort{
							{
								ContainerPort: 3000,
								Protocol:      apiv1.ProtocolTCP,
							},
						},
					},
				},
				RestartPolicy: apiv1.RestartPolicyNever,
			},
		}

		queue <- podReq
	}

	return queue
}
