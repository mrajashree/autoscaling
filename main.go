package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	// "github.com/rancher/go-rancher/client"

	"github.com/mrajashree/autoscaling/service"
)

func main() {
	http.HandleFunc("/favicon.ico", handlerICon)
	http.HandleFunc("/", Monitor)
	http.ListenAndServe(":8087", nil)
}

func handlerICon(w http.ResponseWriter, r *http.Request) {}

func Monitor(w http.ResponseWriter, r *http.Request) {
	requestContent, err := ioutil.ReadFile("scaleUp.json")
	if err != nil {
		fmt.Printf("Error : %v\n", err)
	}
	autoScalePolicies, err := service.CreateWebhook(requestContent)
	if err != nil {
		fmt.Printf("Error : %v\n", err)
	}
	fmt.Printf("autoScalePolicies : %v\n", autoScalePolicies)

	err = service.GetContainers(autoScalePolicies)
	if err != nil {
		fmt.Println(err)
	}
}
