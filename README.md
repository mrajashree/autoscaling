<h1> Autoscaling </h1>

<h2> Design </h2>
<h3> Collecting Stats </h3>
- Rancher-Autoscaling microservice starts as a separate service like webhook with cattle. 
- User through UI selects 'autoscale' option for a service, enters the rules for autoscaling. It will include the following parameters: </br>

serviceId | metric | threshold | min | max | action | amount | webhook | quietPeriod | lastExecuted
--- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- 
string | string | int | int | int | string | int | string | int | int

This will be saved as a resource called `AutoScalePolicy`. <br>
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
**Approach 1**
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

**Approach 2**
- Kubernetes HorizontalPodAutoscaler calculates CPUUsage% as 
`(CPUUtilization avg over 1 last minute)/(CPU requested by the pod) * 100`
- The CPU request is in mCPU for kubernetes. Request in terms of mCPU can be provided through the Rancher UI while creating a service. Kubernetes makes it a must to specify CPU requests while creating pods/containers, or the kubernetes HorizontalPodAutoscaler ignores the containers during autoscaling for which the CPU request is not specified. Because the CPUUtilization is with respect to the CPU requested. Kubernetes uses heapster for getting metrics and it has a default [metric resolution](https://github.com/kubernetes/heapster/blob/e19c9fb0d78695ce02e34afc821be5525f70f1d7/metrics/options/options.go#L56) of 60s. The HorizontalPodAutoscaler period is 30s by default.

<h3> Executing scale up/down </h3>
- The CPUUtilization will be compared with threshold supplied during creation of AutoScalePolicy. The containerStats are incoming at a rate of 1 entry per second for all containers of a service
```
	var containerStats map[string]Stats	//Stats will contain fields necessary for utilization calculation
	
	func CPUUtilizationThresholdCrossed (threshold) bool {
		currentCPUUtilization := CalculateCPUUsage()
		if currentCPUUtilization > threshold {
		currentTime := Time.Now()
			if (currentTime - autoScale.lastExecuted) > autoScale.quietPeriod {
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
			avgCPUUtilization := CPUUtilization / len(externalIds) //To get avgCPUUtilization of the service
			CPUUtilizationSixtySeconds = append(CPUUtilizationSixtySeconds, avgCPUUtilization)
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

where the first key `id` is the container's externalId obtained in step 2. It will store this in a map
```
	var containerStats map[string]Stats
	
	type Stats struct {
		cpuTotal	int64
		cpuPer		int64
		systemUsage	int64
		memory  	int64
	}
	
	containerStats[id].cpuTotal = cpu.usage.total
	containerStats[id].cpuPer = cpu.usage.per_cpu_usage
	containerStats[id].systemUsage = cpu.system_usage // This is not present in stats currently

```
- The autoscaler will check the service containers every 30s using following formula:
```
	autoscaleFlag = false
	for 1 minute
		averageCPUUsed := (CPUUsage(container1)+CPUUsage(container2)+CPUUsage(container3))/3
		if averageCPUUsed > thresholdCPU {
			autoscaleFlag = true
		} else {
			autoscaleFlag = false
		}
	end for
	
	if autoscaleFlag == true {
		if autoScale.executed == true {
			if (currentTime - autoScale.lastExecuted) > autoScale.quietPeriod {
				POST(autoScale.webhook)
			}
		}
	}
	
```

<h2> Initial description </h2>

* There will be one autoscaler running per service. The autoscaler will need the cattle API keys to make changes to the scale of the service. It will have three actions for scaling:</br>
Scale out: Increase number of containers of the service</br>
Scale in: Decrease number of containers of the service</br>
Default: Set to the default number of containers, with which the service was created</br>

* The cpu shares and memory limit for every container need to be specified when it is created. If these fields are not specified then the autoscaler will not know the current % CPU utilization or % memory utilization and won’t take any action on that service. So the fields `cpu_shares` and `mem_limit` should be set for the service in `docker-compose.yml` file.

* User can add rules as following:</br>
If average CPU utilization >= 60% for 10 minutes, scale out by 1 container</br>
			   >= 85% for 5 minutes, scale out by 3 containers</br>
			   <= 40% for 25 minutes, scale in by 2 containers</br>
Each rule will be of the form:</br>
`If <metric> <operation> <metric_value> <time_span> then <scale_action> <number_of_containers>`, where</br>
<b> metric </b> can be one of the three: % CPU utilization, % memory utilization and HTTP/TCP requests/second</br>
<b> operation </b> can be one of these: ‘<=’, ‘>=’, ‘=’, ‘<’, ‘>’</br>
<b> metric_value </b> will be specified by the user. It will be the percentage of the threshold value of that metric, at which scale in or scale out should happen.</br>
<b> time_span </b> is the time for which the containers need to have the `<metric_value>` before scale in or scale out takes place. Time span for scale out operations should be more than that for scale in.</br>
<b> scale_action </b> can be one of these: Scale out, Scale in, Default (set to original number of containers)</br>
<b> number_of_containers </b> is the change in number of containers.</br>

* Autoscaler can obtain CPU and memory utilization through API, and get the HTTP request rate from HAProxy Frontend Metrics
