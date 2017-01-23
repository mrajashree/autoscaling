package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	// "os"
	// "time"

	"github.com/gorilla/websocket"
	"github.com/mrajashree/autoscaling/types"
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
	// GetHAProxy(projectID, serviceId, apiClient)
	err = GetStats(externalIds, projectID, serviceId, apiClient)
	if err != nil {
		return nil, err
	}
	return externalIds, nil
}

func GetStats(externalIds []string, projectID string, serviceId string, apiClient client.RancherClient) error {
	// fmt.Printf("externalIds : %v\n", externalIds)
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

	currentMemUtilization := make(chan float64)

	go CalculateStats(service, projectID, conn, currentMemUtilization)
	val := <-currentMemUtilization
	fmt.Printf("VAL!!!!!!! : %v\n", val)
	return nil
}

func CalculateStats(service *client.Service, projectID string, conn *websocket.Conn, currentMemUtilization chan float64) error {
	apiClient, err := GetClient(projectID)
	if err != nil {
		return err
	}

	var MemUtilThirtySeconds []float64
	var avgMemUtilization float64

	for {
		counter := 0
		containerCount := len(service.InstanceIds)
		MemUtilService := float64(0)
		for counter < containerCount {
			_, buffer, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("Error in readMessage: %v", err)
			}
			var arr []types.ContainerInfoStats
			err = json.Unmarshal(buffer, &arr)
			if err != nil {
				return fmt.Errorf("Error in marshal: %v", err)
			}
			fmt.Printf("ID : %v\n", arr[0].ID)
			memUsed := int64(arr[0].Memory.Usage)
			fmt.Printf("Memory used : %v\n", memUsed)

			fmt.Printf("CPU : %v\n", arr[0].CPU.Usage)
			container, err := apiClient.Container.ById(service.InstanceIds[counter])
			if err != nil {
				return fmt.Errorf("Error in get Container: %v", err)
			}
			if container == nil {
				return fmt.Errorf("Container not found")
			}
			memReserved := container.MemoryReservation
			if memReserved == 0 {
				continue
			}
			fmt.Printf("Memory requested : %v\n", memReserved)
			memoryUtilization := (float64(memUsed) / float64(memReserved)) * 100
			fmt.Printf("MemoryUtilization : %v\n\n", memoryUtilization)
			MemUtilService += memoryUtilization
			counter++
		}
		MemUtilService = MemUtilService / float64(len(service.InstanceIds))
		MemUtilThirtySeconds = append(MemUtilThirtySeconds, MemUtilService)
		fmt.Println(len(MemUtilThirtySeconds))
		if len(MemUtilThirtySeconds) == 30 {
			avgMemUtilization = sum(MemUtilThirtySeconds...) / 30
			currentMemUtilization <- avgMemUtilization
		}
	}
	return nil
}

func sum(nums ...float64) float64 {
	total := float64(0)
	for _, num := range nums {
		total += num
	}
	return total
}

// func GetHAProxy(projectID string, serviceId string, apiClient client.RancherClient) error {
// 	resp, err := http.Get("http://nginxLB:9000/haproxy_stats;csv")
// 	if err != nil {
// 		fmt.Println(err)
// 		return nil
// 	}
// 	defer resp.Body.Close()
// 	body, err := ioutil.ReadAll(resp.Body)
// 	fmt.Println(string(body))
// 	return nil
// }
