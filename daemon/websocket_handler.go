package daemon

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sdir/mosesacs/cwmp"
	"golang.org/x/net/websocket"
)

func websocketHandler(ws *websocket.Conn) {
	fmt.Println("New websocket client via ws")
	defer ws.Close()

	client := Client{ws: ws, start: time.Now().UTC()}
	clients = append(clients, client)
	//	client.Read()

	quit := make(chan bool)
	go periodicWsChecker(&client, quit)

	for {
		var msg WsSendMessage
		err := websocket.JSON.Receive(ws, &msg)
		if err != nil {
			fmt.Println("error while Receive:", err)
			quit <- true
			break
		}

		data := make(map[string]interface{})
		err = json.Unmarshal(msg.Data, &data)

		if err != nil {
			fmt.Println("error:", err)
		}

		m := data["command"].(string)

		if m == "list" {

			ms := new(WsSendMessage)
			ms.MsgType = "cpes"
			msgCpes := new(MsgCPEs)
			msgCpes.CPES = cpes
			ms.Data, _ = json.Marshal(msgCpes)

			client.SendNew(ms)

			// client requests a GetParametersValues to cpe with serial
			//serial := "1"
			//leaf := "Device.Time."
			// enqueue this command with the ws number to get the answer back

		} else if m == "version" {
			client.Send(fmt.Sprintf("MosesAcs Daemon %s", Version))

		} else if m == "status" {
			var response string
			for i := range clients {
				response += clients[i].String() + "\n"
			}

			client.Send(response)
		} else if strings.Contains(m, "setxmpp") {
			i := strings.Split(m, " ")
			if _, exists := cpes[i[1]]; exists {
				c := cpes[i[1]]
				c.XmppId = i[2]
				if len(i) == 5 {
					c.XmppUsername = i[3]
					c.XmppPassword = i[4]
				}
				cpes[i[1]] = c
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", i[1]))
			}
		} else if strings.Contains(m, "changeDuState") {
			i := strings.Split(m, " ")

			ops := data["ops"].([]interface{})

			var operations []fmt.Stringer
			for _, obj := range ops {
				op := obj.(map[string]interface{})
				fmt.Println(op)
				type_cmd := op["type"].(string)
				if type_cmd == "install" {
					install_op := &cwmp.InstallOpStruct{Url: op["url"].(string), Uuid: op["uuid"].(string), Username: op["username"].(string), Password: op["password"].(string), ExecutionEnvironment: op["environment"].(string)}
					operations = append(operations, install_op)
				} else if type_cmd == "update" {
					update_op := &cwmp.UpdateOpStruct{Url: op["url"].(string), Uuid: op["uuid"].(string), Username: op["username"].(string), Password: op["password"].(string), Version: op["version"].(string)}
					operations = append(operations, update_op)
				} else if type_cmd == "uninstall" {
					uninstall_op := &cwmp.UninstallOpStruct{Version: op["version"].(string), Uuid: op["uuid"].(string), ExecutionEnvironment: op["environment"].(string)}
					operations = append(operations, uninstall_op)
				}
			}

			req := Request{i[1], ws, cwmp.ChangeDuState(operations), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[i[1]]; exists {
				cpes[i[1]].Queue.Enqueue(req)
				if cpes[i[1]].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(i[1])
				}
			} else {
				if err := websocket.JSON.Send(ws, map[string]string{"status": "error", "reason": fmt.Sprintf("CPE with serial %s not found", i[1])}); err != nil {
					fmt.Println("error while sending back answer:", err)
				}
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", i[1]))
			}
		} else if m == "download" {
			CpeSerial := data["serial"].(string)

			req := Request{CpeSerial, ws, cwmp.Download(data["filetype"].(string), data["url"].(string), data["username"].(string), data["password"].(string), data["filesize"].(string)), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[CpeSerial]; exists {
				cpes[CpeSerial].Queue.Enqueue(req)
				if cpes[CpeSerial].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(CpeSerial)
				}
			} else {
				if err := websocket.JSON.Send(ws, map[string]string{"status": "error", "reason": fmt.Sprintf("CPE with serial %s not found", CpeSerial)}); err != nil {
					fmt.Println("error while sending back answer:", err)
				}
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", CpeSerial))
			}
		} else if strings.Contains(m, "canceltransfer") {
			CpeSerial := data["serial"].(string)

			req := Request{CpeSerial, ws, cwmp.CancelTransfer(), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[CpeSerial]; exists {
				cpes[CpeSerial].Queue.Enqueue(req)
				if cpes[CpeSerial].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(CpeSerial)
				}
			} else {
				if err := websocket.JSON.Send(ws, map[string]string{"status": "error", "reason": fmt.Sprintf("CPE with serial %s not found", CpeSerial)}); err != nil {
					fmt.Println("error while sending back answer:", err)
				}
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", CpeSerial))
			}

		} else if strings.Contains(m, "scheduledownload") {
			CpeSerial := data["serial"].(string)

			w := data["windows"].([]interface{})
			var windows []fmt.Stringer
			for _, obj := range w {
				i := obj.(map[string]interface{})
				wdw := &cwmp.TimeWindowStruct{
					WindowStart: i["windowstart"].(string),
					WindowEnd:   i["windowend"].(string),
					WindowMode:  i["windowmode"].(string),
					UserMessage: i["usermessage"].(string),
					MaxRetries:  i["maxretries"].(string),
				}
				windows = append(windows, wdw)
			}

			req := Request{CpeSerial, ws, cwmp.ScheduleDownload(data["filetype"].(string), data["url"].(string), data["username"].(string), data["password"].(string), data["filesize"].(string), windows), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[CpeSerial]; exists {
				cpes[CpeSerial].Queue.Enqueue(req)
				if cpes[CpeSerial].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(CpeSerial)
				}
			} else {
				if err := websocket.JSON.Send(ws, map[string]string{"status": "error", "reason": fmt.Sprintf("CPE with serial %s not found", CpeSerial)}); err != nil {
					fmt.Println("error while sending back answer:", err)
				}
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", CpeSerial))
			}
		} else if strings.Contains(m, "readMib") {
			i := strings.Split(m, " ")
			//			cpeSerial, _ := strconv.Atoi(i[1])
			//			fmt.Printf("CPE %d\n", cpeSerial)
			//			fmt.Printf("LEAF %s\n", i[2])
			req := Request{i[1], ws, cwmp.GetParameterValues(i[2]), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[i[1]]; exists {
				cpes[i[1]].Queue.Enqueue(req)
				if cpes[i[1]].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(i[1])
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", i[1]))
			}

		} else if strings.Contains(m, "writeMib") {
			i := strings.Split(m, " ")
			req := Request{i[1], ws, cwmp.SetParameterValues(i[2], i[3]), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[i[1]]; exists {
				cpes[i[1]].Queue.Enqueue(req)
				if cpes[i[1]].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(i[1])
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", i[1]))
			}
		} else if strings.Contains(m, "GetParameterNames") {
			i := strings.Split(m, " ")
			nextlevel, _ := strconv.Atoi(i[3])
			req := Request{i[1], ws, cwmp.GetParameterNames(i[2], nextlevel), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}

			if _, exists := cpes[i[1]]; exists {
				cpes[i[1]].Queue.Enqueue(req)
				if cpes[i[1]].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(i[1])
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", i[1]))
			}
		} else if m == "GetParameterValues" {
			cpe := data["cpe"].(string)
			req := Request{cpe, ws, cwmp.GetParameterValues(data["object"].(string)), func(msg *WsSendMessage) error {
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}
			if _, exists := cpes[cpe]; exists {
				cpes[cpe].Queue.Enqueue(req)
				if cpes[cpe].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(cpe)
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", cpe))
			}
		} else if m == "GetSummary" {
			cpe := data["cpe"].(string)
			ch := make(chan *WsSendMessage)

			// GetParameterNames per leggere la mib velocemente
			req := Request{cpe, ws, cwmp.GetParameterNames(data["object"].(string), 0), func(msg *WsSendMessage) error {
				fmt.Println("sono nella callback della GetParameterNames")
				ch <- msg
				return nil
			}}
			if _, exists := cpes[cpe]; exists {
				cpes[cpe].Queue.Enqueue(req)
				if cpes[cpe].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(cpe)
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", cpe))
			}
			fmt.Println("I'm suspended waiting for you to return the message")
			m := <-ch
			fmt.Println("is back")
			getParameterNames := new(cwmp.GetParameterNamesResponse)
			err := json.Unmarshal(m.Data, &getParameterNames)
			if err != nil {
				fmt.Println("error:", err)
			}

			objectsToCheck := map[string][]string{}
			re_wan_ip := regexp.MustCompile(`InternetGatewayDevice.WANDevice.(\d+).WANConnectionDevice.(\d+).WANIPConnection.(\d+).(Name|ExternalIPAddress|Enable|NATEnabled|Username|ConnectionTrigger|AddressingType|DefaultGateway|ConnectionType|ConnectionStatus)`)
			re_wan_ppp := regexp.MustCompile(`InternetGatewayDevice.WANDevice.(\d+).WANConnectionDevice.(\d+).WANPPPConnection.(\d+).(Name|ExternalIPAddress|Enable|ConnectionTrigger|AddressingType|DefaultGateway|ConnectionType|ConnectionStatus)`)
			re_hosts := regexp.MustCompile(`InternetGatewayDevice.LANDevice.1.Hosts.Host.(\d+).(Active|HostName|IPAddress|MACAddress|InterfaceType)`)
			re_wifi := regexp.MustCompile(`InternetGatewayDevice.LANDevice.1.WLANConfiguration.(\d+).(SSID|Enable|Status)`)
			re_voice := regexp.MustCompile(`InternetGatewayDevice.Services.VoiceService.(\d+).VoiceProfile.(\d+).Line.(\d+).(SIP.AuthUserName|SIP.URI|Enable|Status)`)
			// parso la GetParameterNamesResponse per creare la GetParameterValues multipla con le sole foglie che interessano il summary
			for idx := range getParameterNames.ParameterList {
				// looking for WAN IPConnection
				// InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANIPConnection.2.DefaultGateway
				match := re_wan_ip.FindStringSubmatch(getParameterNames.ParameterList[idx].Name)
				if len(match) != 0 {
					objectsToCheck["WAN device "+match[1]+" connection "+match[2]+" IP connection "+match[3]] = append(objectsToCheck["WAN device "+match[1]+" connection "+match[2]+" IP connection "+match[3]], "InternetGatewayDevice.WANDevice."+match[1]+".WANConnectionDevice."+match[2]+".WANIPConnection."+match[3]+"."+match[4])
				}

				// looking for WAN PPPConnection
				match = re_wan_ppp.FindStringSubmatch(getParameterNames.ParameterList[idx].Name)
				if len(match) != 0 {
					objectsToCheck["WAN device "+match[1]+" connection "+match[2]+" PPP connection "+match[3]] = append(objectsToCheck["WAN device "+match[1]+" connection "+match[2]+" PPP connection "+match[3]], "InternetGatewayDevice.WANDevice."+match[1]+".WANConnectionDevice."+match[2]+".WANPPPConnection."+match[3]+"."+match[4])
				}

				// looking for LAN
				match = re_hosts.FindStringSubmatch(getParameterNames.ParameterList[idx].Name)
				if len(match) != 0 {
					objectsToCheck["HOST"+match[1]] = append(objectsToCheck["HOST"+match[1]], "InternetGatewayDevice.LANDevice.1.Hosts.Host."+match[1]+"."+match[2])
				}
				// looking for WIFI
				match = re_wifi.FindStringSubmatch(getParameterNames.ParameterList[idx].Name)
				if len(match) != 0 {
					objectsToCheck["WIFI"+match[1]] = append(objectsToCheck["WIFI"+match[1]], "InternetGatewayDevice.LANDevice.1.WLANConfiguration."+match[1]+"."+match[2])
				}
				// looking for VOICE
				match = re_voice.FindStringSubmatch(getParameterNames.ParameterList[idx].Name)
				if len(match) != 0 {
					objectsToCheck["VOICE "+match[1]+" profile "+match[2]+" line "+match[3]] = append(objectsToCheck["VOICE "+match[1]+" profile "+match[2]+" line "+match[3]], "InternetGatewayDevice.Services.VoiceService."+match[1]+".VoiceProfile."+match[2]+".Line."+match[3]+"."+match[4])
				}

			}

			// GetParameterMultiValues
			leaves := []string{}
			for idx := range objectsToCheck {
				for i := range objectsToCheck[idx] {
					leaves = append(leaves, objectsToCheck[idx][i])
				}
			}
			req = Request{cpe, ws, cwmp.GetParameterMultiValues(leaves), func(msg *WsSendMessage) error {
				fmt.Println("sono nella callback")
				ch <- msg
				return nil // TODO da implementare un timeout ? boh
			}}
			if _, exists := cpes[cpe]; exists {
				cpes[cpe].Queue.Enqueue(req)
				if cpes[cpe].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(cpe)
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", cpe))
			}

			fmt.Println("i'm waiting for the message to be back")
			m = <-ch
			fmt.Println("it's back")

			// qui devo parsare la response e creare il summary "semplice" da visualizzare
			getParameterValues := new(cwmp.GetParameterValuesResponse)
			err = json.Unmarshal(m.Data, &getParameterValues)
			if err != nil {
				fmt.Println("error:", err)
			}

			summaryObject := map[string]map[string]string{}
			for area := range objectsToCheck {
				summaryObject[area] = make(map[string]string)
			}

			for idx := range getParameterValues.ParameterList {
				objectName := getParameterValues.ParameterList[idx].Name

				for area := range objectsToCheck {
					for leafIndex := range objectsToCheck[area] {
						leaf := objectsToCheck[area][leafIndex]
						if objectName == leaf {
							// leafName := strings.Split(leaf, ".")
							// summaryObject[area][leafName[len(leafName)-1]] = getParameterValues.ParameterList[idx].Value
							summaryObject[area][leaf] = getParameterValues.ParameterList[idx].Value
						}
					}
				}
			}

			m.MsgType = "SummaryResponse"
			dataSummary := map[string]map[string]string{}
			for area := range objectsToCheck {
				dataSummary[area] = summaryObject[area]
			}

			m.Data, _ = json.Marshal(dataSummary)

			if err := websocket.JSON.Send(ws, m); err != nil {
				fmt.Println("error while sending back answer:", err)
			}

		} else if m == "getMib" {
			cpe := data["cpe"].(string)
			req := Request{cpe, ws, cwmp.GetParameterNames(data["object"].(string), 1), func(msg *WsSendMessage) error {
				fmt.Println("sono nella callback")
				if err := websocket.JSON.Send(ws, msg); err != nil {
					fmt.Println("error while sending back answer:", err)
				}

				return err
			}}
			if _, exists := cpes[cpe]; exists {
				cpes[cpe].Queue.Enqueue(req)
				if cpes[cpe].State != "Connected" {
					// issue a connection request
					go doConnectionRequest(cpe)
				}
			} else {
				fmt.Println(fmt.Sprintf("CPE with serial %s not found", cpe))
			}
		}
	}
	fmt.Println("ws closed, leaving read routine")

	for i := range clients {
		if clients[i].ws == ws {
			clients = append(clients[:i], clients[i+1:]...)
		}
	}
}

func sendAll(msg string) {
	for i := range clients {
		clients[i].Send(msg)
	}
}

func periodicWsChecker(c *Client, quit chan bool) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			//			fmt.Println("new tick on client:", c)
			c.Send("ping")
		case <-quit:
			fmt.Println("received quit command for periodicWsChecker")
			ticker.Stop()
			return
		}
	}
}
