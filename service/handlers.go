package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mrajashree/autoscaling/types"
	"github.com/rancher/go-rancher/v2"
)

func GetContainers(autoScalePolicy []AutoScale) error {
	for _, autoScaleObj := range autoScalePolicy {
		serviceId := autoScaleObj.ServiceID
		projectID := autoScaleObj.ProjectId
		apiClient, err := GetClient(projectID)
		if err != nil {
			return err
		}

		service, err := apiClient.Service.ById(serviceId)
		if err != nil {
			return fmt.Errorf("Error in GetContainers for getService")
		}
		if service == nil || service.Removed != "" {
			return fmt.Errorf("service %s not found", serviceId)
		}
		var externalIds []string
		for _, instanceId := range service.InstanceIds {
			instance, _ := apiClient.Instance.ById(instanceId)
			externalIds = append(externalIds, instance.ExternalId)
		}
		// GetHAProxy(projectID, serviceId, apiClient)
		err = GetStats(projectID, serviceId, apiClient, autoScaleObj)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetStats(projectID string, serviceId string, apiClient client.RancherClient, autoScaleObj AutoScale) error {
	currentMemUtilization := make(chan float64, 1)
	go CalculateStats(serviceId, projectID, currentMemUtilization)
	ticker := time.NewTicker(time.Second * 15)
	go func() {
		for t := range ticker.C {
			fmt.Printf("Getting calculated stats on channel at time %s\n", t)
			val := <-currentMemUtilization
			compareWithThreshold(val, autoScaleObj)
			fmt.Printf("Memory : %v\n", val)
		}
	}()
	return nil
}

func compareWithThreshold(currentMemUtilization float64, autoScaleObj AutoScale) error {
	if autoScaleObj.Action == "up" {
		if currentMemUtilization > autoScaleObj.Threshold {
			http.Post(autoScaleObj.Webhook, "", nil)
		}
	} else {
		if currentMemUtilization < autoScaleObj.Threshold {
			http.Post(autoScaleObj.Webhook, "", nil)
		}
	}

	return nil
}

func CalculateStats(serviceId string, projectID string, currentMemUtilization chan float64) error {
	apiClient, err := GetClient(projectID)
	if err != nil {
		return err
	}

	conn := openWebsocket(serviceId, apiClient)

	MemUtilChannel := make(chan float64, 10)
	var avgMemUtilization float64
	var MemUtilTotal float64

	service, err := apiClient.Service.ById(serviceId)
	if err != nil {
		return err
	}
	if service == nil || service.Removed != "" {
		return fmt.Errorf("Service not found/deleted")
	}
	previousLen := len(service.InstanceIds)

	for {
		counter := 0
		service, err := apiClient.Service.ById(serviceId)
		if err != nil {
			return err
		}
		if service == nil || service.Removed != "" {
			return fmt.Errorf("Service not found/deleted")
		}

		if previousLen != len(service.InstanceIds) {
			conn.Close()
			conn = openWebsocket(serviceId, apiClient)
		}

		MemUtilService := float64(0)
		for counter < len(service.InstanceIds) {
			_, buffer, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("Error in readMessage: %v", err)
			}
			var arr []types.ContainerInfoStats
			err = json.Unmarshal(buffer, &arr)
			if err != nil {
				return fmt.Errorf("Error in marshal: %v", err)
			}
			memUsed := int64(arr[0].Memory.Usage)
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
			memoryUtilization := (float64(memUsed) / float64(memReserved)) * 100
			MemUtilService += memoryUtilization
			counter++
		}
		MemUtilService = MemUtilService / float64(len(service.InstanceIds))
		MemUtilChannel <- MemUtilService

		MemUtilTotal += MemUtilService
		avgMemUtilization = MemUtilTotal / 10

		// Keep calculating current average
		currentMemUtilization <- avgMemUtilization

		if len(MemUtilChannel) == 10 {
			previosMem := <-MemUtilChannel
			MemUtilTotal -= previosMem
		}
		select {
		case <-currentMemUtilization:
			// fmt.Printf("Channel has contents\n")
		default:
			// fmt.Printf("Channel empty\n")
		}

	}
	return nil
}

func openWebsocket(serviceId string, apiClient client.RancherClient) *websocket.Conn {
	service, err := apiClient.Service.ById(serviceId)
	if err != nil {
		return nil
	}
	containerStatsURL := service.Links["containerStats"]
	resp, err := http.Get(containerStatsURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	respMap := make(map[string]interface{})
	err = json.Unmarshal(body, &respMap)
	if err != nil {
		return nil
	}

	websocketURL := respMap["url"].(string) + "?token=" + respMap["token"].(string)
	requestHeader := http.Header{}
	requestHeader.Add("Connection", "Upgrade")
	requestHeader.Add("Upgrade", "websocket")
	requestHeader.Add("Content-type", "application/json")
	conn, _, err := websocket.DefaultDialer.Dial(websocketURL, requestHeader)
	return conn
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
