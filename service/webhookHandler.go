package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	// "time"

	"github.com/rancher/webhook-service/model"
)

type AutoScale struct {
	ServiceID    string  `json:"serviceId,omitempty" mapstructure:"serviceId"`
	Metric       string  `json:"metric,omitempty" mapstructure:"metric"`
	Threshold    float64 `json:"threshold,omitempty" mapstructure:"threshold"`
	Amount       int64   `json:"amount,omitempty" mapstructure:"amount"`
	Action       string  `json:"action,omitempty" mapstructure:"action"`
	Min          int64   `json:"min,omitempty" mapstructure:"min"`
	Max          int64   `json:"max,omitempty" mapstructure:"max"`
	ProjectId    string  `json:"ProjectId,omitempty" mapstructure:"ProjectId"`
	Webhook      string  `json:"webhook,omitempty" mapstructure:"webhook"`
	LastExecuted int     `json:"lastExecuted,omitempty" mapstructure:"lastExecuted"`
	QuietPeriod  int64   `json:"quietPeriod,omitempty" mapstructure:"quietPeriod"`
}

var AutoScalePolicy []AutoScale

type WebhookConstructData struct {
	Name               string             `json:"name,omitempty" mapstructure:"name"`
	Driver             string             `json:"driver,omitempty" mapstructure:"driver"`
	ScaleServiceConfig model.ScaleService `json:scaleServiceConfig,omitempty" mapstructre:"scaleServiceConfig"`
}

func CreateWebhook(requestData []byte) ([]AutoScale, error) {
	err := json.Unmarshal(requestData, &AutoScalePolicy)
	if err != nil {
		return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
	}
	// fmt.Printf("AutoScale : %v\n", AutoScalePolicy)

	var scaleServ []model.ScaleService
	err = json.Unmarshal(requestData, &scaleServ)
	if err != nil {
		return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
	}
	// fmt.Printf("scaleServ : %v\n", scaleServ)

	config := GetConfig()
	baseURL := config.CattleURL

	for key, autoScaleObj := range AutoScalePolicy {
		var newWebhook WebhookConstructData
		newWebhook.Name = autoScaleObj.ServiceID + autoScaleObj.Metric + autoScaleObj.Action
		newWebhook.Driver = "scaleService"
		newWebhook.ScaleServiceConfig = scaleServ[key]
		webhookBytes, err := json.Marshal(newWebhook)
		if err != nil {
			return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
		}

		webhookConstructURL := strings.Split(baseURL, "v2-beta")[0] + "v1-webhooks/receivers?projectId=" + autoScaleObj.ProjectId
		bytesBody := bytes.NewReader(webhookBytes)
		req, err := http.NewRequest("POST", webhookConstructURL, bytesBody)
		if err != nil {
			return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
		}
		respBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
		}
		respMap := make(map[string]interface{})
		err = json.Unmarshal(respBytes, &respMap)
		if err != nil {
			return []AutoScale{}, fmt.Errorf("Err : %v\n", err)
		}
		autoScaleObj.Webhook = respMap["url"].(string)
		AutoScalePolicy[key] = autoScaleObj
	}
	return AutoScalePolicy, nil
}
