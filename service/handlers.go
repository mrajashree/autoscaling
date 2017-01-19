package service

import (
	"fmt"
	"os"
	"time"

	"github.com/rancher/go-rancher/v2"
	// "github.com/gorilla/websocket"
)

func GetContainers(parameters map[string]interface{}) (map[string]interface{}, error) {
	serviceId := parameters["serviceId"].(string)
	projectID := parameters["projectId"].(string)
	apiClient, err := GetClient(projectID)
	if err != nil {
		return nil, err
	}

	service, err := apiClient.Service.ById(serviceId)
	for _, instanceId := range service.InstanceIds {
		instance, _ := apiClient.Instance.ById(instanceId)
		fmt.Printf("externalId : %v\n", instance.ExternalId)
		fmt.Printf("actions : %v\n", instance.Actions)
	}
	return parameters, nil
}

func GetStats() {

}

type Config struct {
	CattleURL       string
	CattleAccessKey string
	CattleSecretKey string
}

func GetConfig() Config {
	config := Config{
		CattleURL:       os.Getenv("CATTLE_URL"),
		CattleAccessKey: os.Getenv("CATTLE_ACCESS_KEY"),
		CattleSecretKey: os.Getenv("CATTLE_SECRET_KEY"),
	}

	return config
}

func GetClient(projectID string) (client.RancherClient, error) {
	config := GetConfig()
	url := fmt.Sprintf("%s/projects/%s/schemas", config.CattleURL, projectID)
	apiClient, err := client.NewRancherClient(&client.ClientOpts{
		Timeout:   time.Second * 30,
		Url:       url,
		AccessKey: config.CattleAccessKey,
		SecretKey: config.CattleSecretKey,
	})
	if err != nil {
		return client.RancherClient{}, fmt.Errorf("Error in creating API client")
	}
	return *apiClient, nil
}
