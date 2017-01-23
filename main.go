package main

import (
	"encoding/json"
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

type WebhookRequest map[string]interface{}

func Monitor(w http.ResponseWriter, r *http.Request) {
	var webhookRequestData WebhookRequest
	requestContent, err := ioutil.ReadFile("scaleUp.json")
	if err != nil {
		fmt.Printf("Error : %v\n", err)
	}
	// err = service.CreateWebhook(requestContent)
	// if err != nil {
	// 	fmt.Printf("Error : %v\n", err)
	// }
	json.Unmarshal(requestContent, &webhookRequestData)

	_, err = service.GetContainers(webhookRequestData)
	if err != nil {
		fmt.Println(err)
	}
	// service.GetStats(externalIds)
}
