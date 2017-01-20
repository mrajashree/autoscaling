package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/go-rancher/v2"
)

func GetContainers(parameters map[string]interface{}) ([]string, error) {
	serviceId := parameters["serviceId"].(string)
	projectID := parameters["projectId"].(string)
	apiClient, err := GetClient(projectID)
	if err != nil {
		return nil, err
	}

	service, err := apiClient.Service.ById(serviceId)
	if err != nil {
		return nil, fmt.Errorf("Error in GetContainers for getService")
	}
	if service == nil || service.Removed != "" {
		return nil, fmt.Errorf("service %s not found", serviceId)
	}
	var externalIds []string
	for _, instanceId := range service.InstanceIds {
		instance, _ := apiClient.Instance.ById(instanceId)
		externalIds = append(externalIds, instance.ExternalId)
	}
	err = GetStats(externalIds, projectID, serviceId, apiClient)
	if err != nil {
		return nil, err
	}
	return externalIds, nil
}

func GetStats(externalIds []string, projectID string, serviceId string, apiClient client.RancherClient) error {
	fmt.Printf("externalIds : %v\n", externalIds)
	service, err := apiClient.Service.ById(serviceId)
	if err != nil {
		return err
	}
	containerStatsURL := service.Links["containerStats"]
	resp, err := http.Get(containerStatsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	respMap := make(map[string]interface{})
	err = json.Unmarshal(body, &respMap)
	if err != nil {
		return err
	}

	websocketURL := respMap["url"].(string) + "?token=" + respMap["token"].(string)
	requestHeader := http.Header{}
	requestHeader.Add("Connection", "Upgrade")
	requestHeader.Add("Upgrade", "websocket")
	requestHeader.Add("Content-type", "application/json")
	conn, resp, err := websocket.DefaultDialer.Dial(websocketURL, requestHeader)

	for {
		counter := 0
		containerCount := len(externalIds)
		for counter < containerCount {
			_, buffer, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("Error in readMessage: %v", err)
			}
			var arr []map[string]interface{}
			err = json.Unmarshal(buffer, &arr)
			if err != nil {
				return fmt.Errorf("Error in marshal: %v", err)
			}
			fmt.Printf("Arr : %v\n", arr)
			counter++
		}
		fmt.Printf("\n1 sec done\n\n")
	}

	return nil
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
	fmt.Printf("url : %v\n", url)
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