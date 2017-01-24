<h1> Autoscaling </h1>

<h2> Design </h2>
<h3> Collecting Stats </h3>
- Rancher-Autoscaling microservice starts as a separate service like webhook with cattle. 
- User through UI selects 'autoscale' option for a service, enters the rules for autoscaling. The rule will be saved as a resource called `AutoScalePolicy`. It will include the following parameters: </br>

```
"resourceFields": {
	"serviceId": {
		"type": "string"
	},
	"metric": {
		"type": "enum",
		"options": [
			"CPUUtilizationPercent",
			"MemoryUtilization"
		]
	},
	"threshold": {
		"type": "int"
	}
	"min": {
		"type": "int"
	},
	"max": {
		"type": "int"
	},
	"action": {
		"type": "enum",
		"options": [
			"up",
			"down"
		]
	},
	"amount": {
		"type": "int"
	},
	"webhook": {
		"type": "string"
	},
	"quietPeriod": {
		"type": "int"
	},
	"lastExecuted": {
		"type": "int" //timestamp
	}
}
```
- When a service is created with the option of 'autoscale' selected, a POST request will be sent to Rancher-Autoscaling service with all the above data.
- Using the serviceId, Rancher-Autoscaling service will open a websocket to obtain containerStats.

<h3> Rancher-Autoscaling flow </h3>
- The rancher-autoscale service will receive a POST request from cattle when a autoScalePolicy for a service is added. Using the data from this POST request, a webhook for scale up/down will be created for the service to be autoscaled. Rancher-autoscale service will then update the `AutoScalePolicy` object's webhook field to store the TriggerURL. Also, quietPeriod will be updated to be 3 mins for scaleUp and 5 mins for scaleDown
- Using go-rancher client, the autoscale service will obtain the instanceIds of the service, and externalIds of those instances.
```
	service, err := apiClient.Service.ById(serviceId)
	var externalIds []string
	for _, instanceId := range service.InstanceIds {
		instance, _ := apiClient.Instance.ById(instanceId)
		externalIds = append(externalIds, instance.ExternalId)
	}
	err = GetStats(externalIds, projectID, serviceId, apiClient)
```
- Autoscale service will then send a GET request on containerStats URL of the service to get the websocket URL and statsAccess token
```
	containerStatsURL := service.Links["containerStats"]
	resp, err := http.Get(containerStatsURL)
```
- Using these, it will open a websocket connection to get stats. 
```
	websocketURL := respMap["url"].(string) + "?token=" + respMap["token"].(string)
	conn, resp, err := websocket.DefaultDialer.Dial(websocketURL, requestHeader)
```
- The stats received are of the following form:
```
[map[id:ff16f87ec9ab3154966ffc85a578850f48f3a8957a5fa70b0dadfecc38227872 resourceType:container memLimit:1.044463616e+09
timestamp:2017-01-20T19:11:04.539863676Z cpu:map[usage:map[per_cpu_usage:[6.7905753e+07] user:1e+07 system:1e+07
total:6.7905753e+07]] diskio:map[] network:map[tx_bytes:0 tx_errors:0 interfaces:[map[rx_bytes:5136 rx_packets:74 rx_errors:0
tx_dropped:0 name:eth0 rx_dropped:0 tx_bytes:258 tx_packets:3 tx_errors:0]] name: rx_bytes:0 rx_packets:0 rx_errors:0
rx_dropped:0 tx_packets:0 tx_dropped:0] memory:map[usage:520192]]]
```
**CPU**</br>
*Approach 1*
- Rancher uses docker-stats to get metric values of the containers, so rancher-autoscale service can calculate CPUUtilization% as displayed in the output of `docker stats` command </br>

CONTAINER  | CPU % | MEM USAGE / LIMIT | MEM % | NET I/O | BLOCK I/O | PIDS
--- | --- | --- | --- | --- | --- | ---
d3dd17018b2a | 0.00% | 6.809 MiB / 995.8 MiB | 0.68% | 0 B / 0 B | 3.54 MB / 0 B | 23

Autoscale service can use the docker-stats formula for that:
```
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CPUStats.CPUUsage.TotalUsage) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(v.CPUStats.SystemUsage) - float64(previousSystem)
	)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return cpuPercent
```
- We can calculate the cpuDelta because we get the fields for `CPUUsage` in our stats. But for calculating systemDelta, rancher agent can populate and send [SystemCPUUsage](https://github.com/rancher/agent/blob/master/service/hostapi/stats/common.go#L176) while creating [containerStats output](https://github.com/rancher/agent/blob/0ae80b3770320680a23d2fe41edd08fa11ccdced/service/hostapi/stats/stats_unix.go#L11). Or CPUUsage % calculation can be done in agent.
- Rancher-autoscale service will check the CPUUtilization periodically. This period can be controlled by a flag called `--Autoscale-sync-period` set at the launch of rancher-autoscale service, but will be the same for all AutoScalePolicies. Default will be 30s.

*Approach 2*
- Kubernetes HorizontalPodAutoscaler calculates CPUUsage% as 
`(CPUUtilization avg over 1 last minute)/(CPU requested by the pod) * 100`
- The CPU request is in mCPU for kubernetes. Request in terms of mCPU can be provided through the Rancher UI while creating a service. Kubernetes makes it a must to specify CPU requests while creating pods/containers, or the kubernetes HorizontalPodAutoscaler ignores the containers during autoscaling for which the CPU request is not specified. Because the CPUUtilization is with respect to the CPU requested. Kubernetes uses heapster for getting metrics and it has a default [metric resolution](https://github.com/kubernetes/heapster/blob/e19c9fb0d78695ce02e34afc821be5525f70f1d7/metrics/options/options.go#L56) of 60s. The HorizontalPodAutoscaler period is 30s by default.

<h3> Executing scale up/down </h3>
- The CPUUtilization will be compared with the threshold supplied during creation of AutoScalePolicy. This comparison will take place in `CPUUtilizationThresholdCrossed` function, that calls `CalculateCPUUsage()`. 
- In the function `CalculateCPUUsage()`, stats for all containers of a service are collected every second, and the CPUUtilization is calculated by calling `getCPUUsage()`. At the end of every second, the CPUUtilization for the service is calculated, by taking average CPUUtilization of all containers of the service.
- This will be repeated for 60 seconds, and then the average CPUUtilzation of the service over the last minute is calculated and returned to `CPUUtilizationThresholdCrossed`
- Within `CPUUtilizationThresholdCrossed` function, if the threshold has been crossed, and if the last time the webhook was executed for scaling was before the quietPeriod(3 min or 5 min) amount of time, then the webhook is executed. 

The containerStats are incoming at a rate of 1 entry per second for all containers of a service
```
	var containerStats map[string]Stats	//Stats will contain fields necessary for utilization calculation
	
	func CPUUtilizationThresholdCrossed (threshold) bool {
		currentCPUUtilization := CalculateCPUUsage()
		if currentCPUUtilization > threshold {
		currentTime := Time.Now()
			if (currentTime - autoScale.lastExecuted) > autoScale.quietPeriod {
				autoScale.lastExecuted = currentTime
				http.Post(autoScale.webhook)
			}
		}
	}
	
	func CalculateCPUUsage() {
	CPUUtilizationSixtySeconds := []
		for {
			counter := 0
			containerCount := len(externalIds)
			CPUUtilization := 0
			for counter < containerCount {
				_, buffer, err := conn.ReadMessage()
				if err != nil {
					return fmt.Errorf("Error in readMessage: %v", err)
				}
				var arr []DockerStats	//DockerStats can be struct used for unmarshaling incoming stats
				err = json.Unmarshal(buffer, &arr)
				if err != nil {
					return fmt.Errorf("Error in marshal: %v", err)
				}
				containerStats[arr[id]].CPUTotal = arr.cpu.usage.total //and other required fields
				CPUUtilization += getCPUUsage(containerStats[arr[id]]) 
				counter++
			}
			CPUUtilization = CPUUtilization / len(externalIds) //To get avgCPUUtilization of the service
			CPUUtilizationSixtySeconds = append(CPUUtilizationSixtySeconds, CPUUtilization)
			if len(CPUUtilizationSixtySeconds) == 60 {
				avgCPUUtilizationMinute := CPUUtilizationSixtySeconds / 60
				CPUUtilizationSixtySeconds = []
			}
		}
	}
	
	func getCPUUsage() {
		//Will contain calculations from either of the two approaches
	}
```

**Memory**</br>
To calculate memory percentage as per docker stats, we can use fields `memLimit` and `memory.usage` from the rancher containerStats. 
```
	statsMap := arr[0]
	memLimit := statsMap["memLimit"].(float64)
	memUsed := statsMap["memory"].(map[string]interface{})["usage"].(float64)
	memoryUtilization := (memUsed / memLimit) * 100
```

**Network metrics: Packets dropped**
- Docker [network](https://github.com/docker/docker/blob/801230ce315ef51425da53cc5712eb6063deee95/api/types/stats.go#L119) stats

**Current queued requests, requestes with error connecting, number of connections**
- Every service will have a Rancher LB deployed for it. And the service should have label `lb=<lbName>`. On every host, we can run `curl http://lbName:9000/haproxy_stats;csv` to get the haproxy stats such as [these](https://cbonte.github.io/haproxy-dconv/configuration-1.5.html#9.1)

*Command*
```
cat <(yes | tr \\n x | head -c $((1024*1024*7))) \
<(sleep 120) | grep n
```
