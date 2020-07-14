package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	gomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/prometheus/common/log"
	uuid "github.com/satori/go.uuid"
)

var (
	mqttClient gomqtt.Client
	// clients    []WSClient  = make([]WSClient, 10)
	clients []WSClient  = []WSClient{}
	msgChan chan string = make(chan string)
)

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSClient WSClient
type WSClient struct {
	ID      string
	Conn    *websocket.Conn
	MsgChan chan string
}

// PostMqttForm PostMqttForm
type PostMqttForm struct {
	UserName string `json:"username"`
	Password string `json:"password"`
	Broker   string `json:"broker"`
}

// PostMqttMessageForm PostMqttMessageForm
type PostMqttMessageForm struct {
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

// PostMqttSubForm PostMqttSubForm
type PostMqttSubForm struct {
	Topic string `json:"topic"`
}

func postMqtt(c *gin.Context) {
	var reqInfo PostMqttForm
	c.Bind(&reqInfo)
	fmt.Println(reqInfo)

	if mqttClient != nil && mqttClient.IsConnected() {
		c.JSON(200, gin.H{"errcode": 4000, "errmsg": "已经连接了"})
		return
	}

	opts := gomqtt.NewClientOptions().
		AddBroker(reqInfo.Broker).
		SetPassword(reqInfo.Password).
		SetUsername(reqInfo.UserName).
		SetClientID("go_mqtt-" + time.Now().String()).
		SetConnectTimeout(time.Second * 3)

	mqttClient = gomqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Info("MQTT 连接失败：", token.Error())
		c.JSON(200, gin.H{"errcode": 4000, "data": gin.H{}, "errmsg": token.Error().Error()})
		return
	}
	c.JSON(200, gin.H{"errcode": 2000, "data": gin.H{}})
}

func deleteMqtt(c *gin.Context) {
	defer func() {
		mqttClient = nil
	}()

	if mqttClient != nil && mqttClient.IsConnected() {
		mqttClient.Disconnect(2)

	}
	c.JSON(200, gin.H{"errcode": 2000, "errmsg": "已经断开连接"})

}

func getMqttStatus(c *gin.Context) {
	if mqttClient != nil {
		if mqttClient.IsConnected() {
			c.JSON(200, gin.H{"errcode": 2000, "data": gin.H{"is_connected": true}})
			return
		}

	}

	c.JSON(200, gin.H{"errcode": 2000, "data": gin.H{"is_connected": false}})

}

func sendMessage(c *gin.Context) {

	var reqInfo PostMqttMessageForm
	c.Bind(&reqInfo)

	fmt.Println("reqInfo:", reqInfo)

	if mqttClient == nil {
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": "没有连接"})
		return
	}

	if !mqttClient.IsConnected() {
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": "没有连接"})
		return
	}

	token := mqttClient.Publish(reqInfo.Topic, 0, false, reqInfo.Message)
	token.Wait()

	if token.Error() != nil {
		log.Info("MQTT 发送失败", token.Error())
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": token.Error().Error()})
		return
	}
	c.JSON(200, gin.H{"errcode": 2000, "errmsg": "success"})
}

func subTopic(c *gin.Context) {
	if mqttClient == nil {
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": "没有连接"})
		return
	}

	if !mqttClient.IsConnected() {
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": "没有连接"})
		return
	}

	var reqInfo PostMqttSubForm
	c.Bind(&reqInfo)

	token := mqttClient.Subscribe(reqInfo.Topic, 0, msgCallback)
	token.Wait()

	if token.Error() != nil {
		log.Info("MQTT 订阅", token.Error())
		c.JSON(200, gin.H{"errcode": 4001, "errmsg": token.Error().Error()})
		return
	}
	c.JSON(200, gin.H{"errcode": 2000, "errmsg": "success"})
}

func msgCallback(client gomqtt.Client, message gomqtt.Message) {
	payload := string(message.Payload())

	fmt.Println(payload)
	payload = time.Now().Format("2006-01-02 15:04:05") + " " + message.Topic() + ": " + payload

	for _, client := range clients {
		select {
		case client.MsgChan <- payload:
		}
	}

}

//webSocket请求ping 返回pong
func data(c *gin.Context) {
	//升级get请求为webSocket协议
	ws, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	id := uuid.NewV4().String()
	wsClient := &WSClient{id, ws, make(chan string)}
	clients = append(clients, *wsClient)

	if err != nil {
		return
	}
	defer func() {
		ws.Close()
	}()

	for {
		data := <-wsClient.MsgChan
		fmt.Println(data)
		err = ws.WriteMessage(1, []byte(data))

		if err != nil {
			fmt.Println(err, "delete")
			for index, connClient := range clients {
				if connClient.ID == id {
					clients = append(clients[:index], clients[index+1:]...)
				}
			}
			return
		}
	}
}

func main() {
	r := gin.Default()
	r.RedirectTrailingSlash = false

	r.Use(cors.Default())

	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		c.Redirect(307, "/static/")
	})

	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, map[string]string{"msg": "hello"})
	})

	r.POST("/mqtt", postMqtt)

	r.GET("/mqtt/status", getMqttStatus)

	r.POST("/mqtt/message", sendMessage)

	r.DELETE("/mqtt", deleteMqtt)

	r.POST("/mqtt/subscription", subTopic)

	r.GET("/data", data)

	if os.Getenv("GVM_ADDR") != "" {
		r.Run(os.Getenv("GVM_ADDR"))
	} else {
		r.Run(":9090")
	}

}
