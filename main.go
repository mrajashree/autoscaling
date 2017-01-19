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
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8087", nil)
}

func handlerICon(w http.ResponseWriter, r *http.Request) {}

type WebhookRequest map[string]interface{}

func handler(w http.ResponseWriter, r *http.Request) {
	var webhookRequestData WebhookRequest
	requestContent, err := ioutil.ReadFile("scaleUp.json")
	if err != nil {
		fmt.Printf("Error : %v\n", err)
	}
	json.Unmarshal(requestContent, &webhookRequestData)

	webhookPayload, err := service.GetContainers(webhookRequestData)
	service.GetStats()
	_ = webhookPayload
}
