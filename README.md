
<h1> Autoscaling </h1>

<h2> Design </h2>
<h3> Collecting Stats </h3>
1. Rancher-Autoscaling microservice starts as a separate service like webhook with cattle. 
2. User through UI selects 'autoscale' option for a service, enters the rules for autoscaling. It will include <b>metric, threshold, min, max, action, amount </b> </br>
This will be saved as a resource called `AutoScalePolicy`. <br>
```
"resourceFields": {
	"serviceId": {
		"type": "string"
	},
	"metric": {
		"type": "enum",
		"options": [
			"CPU",
			"Memory"
		]
	},
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
	}
}
```
3. When a service is created with the option of 'autoscale' selected, a POST request will be sent to Rancher-Autoscaling service with all the above data.
4. Using the serviceId, Rancher-Autoscaling service will open a websocket to obtain containerStats.

<h3> Rancher-Autoscaling algorithm </h3>
1. On receiving POST request, a webhook for scale up/down will be created for that service. Update the `AutoScalePolicy` object's webhook field to store the TriggerURL.
2. Using go-rancher client, the autoscale service will obtain the instanceIds of the service, and externalIds of those instances.
```
	service, err := apiClient.Service.ById(serviceId)
	var externalIds []string
	for _, instanceId := range service.InstanceIds {
		instance, _ := apiClient.Instance.ById(instanceId)
		externalIds = append(externalIds, instance.ExternalId)
	}
	err = GetStats(externalIds, projectID, serviceId, apiClient)
```
3. Autoscale service then does a GET request on containerStats URL to get the websocket URL and statsAccess token
```
	containerStatsURL := service.Links["containerStats"]
	resp, err := http.Get(containerStatsURL)
```
Using these, it opens a websocket connection to get stats
```
	websocketURL := respMap["url"].(string) + "?token=" + respMap["token"].(string)
	requestHeader := http.Header{}
	requestHeader.Add("Connection", "Upgrade")
	requestHeader.Add("Upgrade", "websocket")
	requestHeader.Add("Content-type", "application/json")
	conn, resp, err := websocket.DefaultDialer.Dial(websocketURL, requestHeader)
	
	for {
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
	}
```
The stats received are of the following form:
```
[map[id:ff16f87ec9ab3154966ffc85a578850f48f3a8957a5fa70b0dadfecc38227872 resourceType:container memLimit:1.044463616e+09 timestamp:2017-01-20T19:11:04.539863676Z cpu:map[usage:map[per_cpu_usage:[6.7905753e+07] user:1e+07 system:1e+07 total:6.7905753e+07]] diskio:map[] network:map[tx_bytes:0 tx_errors:0 interfaces:[map[rx_bytes:5136 rx_packets:74 rx_errors:0 tx_dropped:0 name:eth0 rx_dropped:0 tx_bytes:258 tx_packets:3 tx_errors:0]] name: rx_bytes:0 rx_packets:0 rx_errors:0 rx_dropped:0 tx_packets:0 tx_dropped:0] memory:map[usage:520192]]]
```
where the first key `id` is the container's externalId. Autoscale service will store these incoming values 

<h2> Steps </h2>
1. Autoscaling microservice starts as a separate service like webhook, catalog with cattle. 
2. User through UI selects 'autoscale' option for a service, enters the scale action, amount, min, max and threshold.
3. Autoscaling service receives POST request when a service with 'autoscale' option selected is created. This POST request has: {serviceId, scaleAction, amount, min, max, threshold}
4. Autoscale service will send POST request to webhook-service to create webhook and save the triggerURL with serviceId
5. Autoscale service uses go-rancher client to get instanceIds and externalIds for the service's containers.
6. It then gets the link for containerStats, sends a GET request to it, and then creates the websocket URL using url and token from the repsonse of GET request.
7. It opens a websocket connection using the above URL, this connection remains open and stats can be read anytime.
8. Algorithm: 
  1. Every 1 minute, the connection is read in a for loop for a minute. We have 'autoscale' flag set to 0 at beginning of the for-loop
  2. Using the previously collected externalIds, we create a map[string]struct. String is the container.externalId and struct has fields for CPU, memory, requests etc.
  3. Within for-loop, we unmarshal read message into the struct and assign to map. Keep calculating avg CPU,         	(CPU.container1 + CPU.container2 + CPU.container3) / 3. If this exceeds the threshold, flip the 'autoscale' flag to on.
  4. If CPU drops below threshold flip it off.
  5. At the end of the loop, if the flag still on, trigger the webhook


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
