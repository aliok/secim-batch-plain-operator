package main

import (
	"log"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"math/rand"
	"time"

	//"github.com/aliok/secim-batch-plain-operator/pkg/secimOperator"
	"flag"
	"path/filepath"
	"os/user"
	"fmt"
)

func main() {
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

	pods, err := k8client.CoreV1().Pods("myproject").List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	podReq := &apiv1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod2",
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
							Value: "5",
						},
						{
							Name:  "BATCH_INDEX",
							Value: "5",
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

	var resp = &apiv1.Pod{}

	if resp, err = k8client.CoreV1().Pods("myproject").Create(podReq); err != nil {
		log.Fatal(err)
		panic(err.Error())
	}

	fmt.Printf("Pod created: %s\n", resp)

	{
		pods, err := k8client.CoreV1().Pods("myproject").List(metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))
	}

}
