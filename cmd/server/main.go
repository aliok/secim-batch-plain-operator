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

	"flag"
	"path/filepath"
	"os/user"
	"math"
	"strconv"
	"io/ioutil"
	"encoding/json"
	"fmt"
)

const PARALLELISM = 16
const BATCH_SIZE = 100
const WORK_ITEM_COUNT = 32000

const JOB_ID = "foo"
const NAMESPACE = "myproject"

const POD_POLLING_INTERVAL = 1 * time.Second

// TODO: improvements
// - env var for constants above
// - env var for in-cluster out-of-cluster config
// - annotations or labels for job pods

var finalResult map[string]interface{}
var finalErrors []string
var queue []*apiv1.Pod
var chunkCount int

func main() {
	finalResult = make(map[string]interface{})
	finalErrors = make([]string, 0)
	chunkCount = int(math.Ceil(float64(WORK_ITEM_COUNT) / float64(BATCH_SIZE)))
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

	createWorkQueue()

	startPollingPodEndpoints(k8client)

	log.Println("Job done, writing results to ./output.json and ./erroredBoxes.txt")
	resultJson, err := json.MarshalIndent(finalResult, "", "  ")
	if err != nil {
		fmt.Println("error marshalling final result:", err)
	}

	errorsJson, err := json.MarshalIndent(finalErrors, "", "  ")
	if err != nil {
		fmt.Println("error marshalling errors:", err)
	}

	err = ioutil.WriteFile("./output.json", resultJson, 0644)
	if err != nil {
		log.Fatalln("Error writing output to file, writing to STDOUT instead")
		log.Println(string(resultJson))
	}

	err = ioutil.WriteFile("./erroredBoxes.txt", errorsJson, 0644)
	if err != nil {
		log.Fatalln("Error writing erroredBoxes to file, writing to STDOUT instead")
		log.Println(string(resultJson))
	}
}

// pull pod list, then check if the pods are done.
// if done, record the output and the errors. then pop one from the queue and create a new pod.
// if not done, don't do anything. it will be checked in the next pull.
func startPollingPodEndpoints(k8client *kubernetes.Clientset) {
	log.Printf("Starting pod endpoint polling")

	startTime := time.Now()
	var podsCreated = 0

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
				// TODO: delete the existing pod first
				queue = append(queue, &pod) // TODO: not sure if this would work . probably not as the UID and the name can't be duplicate
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

		if len(pods.Items) < PARALLELISM && len(queue) > 0 {
			podRequest := queue[0]
			queue = queue[1:]

			// not interested in the response. we're polling them anyway
			if _, err := createPod(podRequest, k8client); err != nil {
				log.Println("Unable to create pod. Gonna readd to the queue")
				log.Fatal(err)
				queue = append(queue, podRequest)
			} else {
				podsCreated++
			}
		}

		if len(pods.Items) == 0 && len(queue) == 0 {
			return
		}

		timeElapsed := time.Since(startTime)
		avgTimeForPod := timeElapsed.Nanoseconds() / int64(podsCreated)
		reminingItemCount := len(queue)
		estimatedRemainingTime := int64(reminingItemCount) * avgTimeForPod

		log.Printf("Queue size: %3d, total pods created: %4d, elapsed time: %s, avg time for a pod: %s, estimated time remaining: %s \n",
			len(queue),
			podsCreated,
			timeElapsed.Truncate(time.Second),
			time.Duration(avgTimeForPod).Truncate(time.Second),
			time.Duration(estimatedRemainingTime).Truncate(time.Second))
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
		log.Printf("Unable to read errors of the Pod %s %s %s . Gonna try again later, perhaps the server in the pod is not up yet", pod.Name, pod.Status.Phase, pod.Status.PodIP)
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

func createPod(podReq *apiv1.Pod, k8client *kubernetes.Clientset) (*apiv1.Pod, error) {
	return k8client.CoreV1().Pods(NAMESPACE).Create(podReq);
}

func createWorkQueue() {
	queue = make([]*apiv1.Pod, 0)

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
						ImagePullPolicy: apiv1.PullAlways,
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

		queue = append(queue, podReq)
	}
}
