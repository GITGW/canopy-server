/*
 * Copyright 2014 Gregory Prisament
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package main

import (
    "fmt"
    "time"
    "encoding/json"
    "code.google.com/p/go.net/websocket"
    "io"
    "net"
    "canopy/datalayer"
    "canopy/datalayer/cassandra_datalayer"
    "canopy/pigeon"
    "canopy/sddl"
)

// Process JSON message from the client
func processPayload(conn datalayer.Connection, payload string, cnt int32) string{
    var payloadObj map[string]interface{}
    var device datalayer.Device
    var deviceIdString string
    var sddlClass *sddl.Class

    err := json.Unmarshal([]byte(payload), &payloadObj)
    if err != nil{
        fmt.Println("Error JSON decoding payload: ", payload)
        return "";
    }

    /* Lookup device */
    _, ok := payloadObj["device_id"]
    if ok {
        deviceIdString, ok = payloadObj["device_id"].(string)
        if !ok {
            fmt.Println("Expected string for device_id")
            return "";
        }

        device, err = conn.LookupDeviceByStringID(deviceIdString)
        if err != nil {
            fmt.Println("Device not found: ", deviceIdString, err)
            return "";
        }
    } else {
            fmt.Println("device-id field mandatory")
            return "";
    }

    /* Store SDDL class */
    _, ok = payloadObj["sddl"]
    if ok {
        sddlJson, ok := payloadObj["sddl"].(map[string]interface{})
        if !ok {
            fmt.Println("Expected object for SDDL")
            return "";
        }
        sddlClass, err = sddl.ParseClass("anonymous", sddlJson)
        if err != nil {
            fmt.Println("Failed parsing sddl class definition: ", err)
            return "";
        }

        err = device.SetSDDLClass(sddlClass)
        if err != nil {
            fmt.Println("Error storing SDDL class during processPayload")
            return "";
        }
    } else {
            fmt.Println("sddl field mandatory")
            return "";
    }


    /* Store sensor data */
    if cnt % 10 == 0 {
        for k, v := range payloadObj {
            /* hack */
            if k == "device_id" || k == "sddl" {
                continue
            }
            sensor, err := sddlClass.LookupSensor(k)
            if err != nil {
                /* sensor not found */
                fmt.Println("Unexpected key: ", k)
                continue
            }
            t := time.Now()
            // convert from JSON to Go
            v2, err := jsonToPropertyValue(sensor, v)
            if err != nil {
                fmt.Println("Warning: ", err)
                continue
            }
            // Insert converts from Go to Cassandra
            err = device.InsertSample(sensor, t, v2)
            if err != nil {
                fmt.Println("Warning: ", err)
                continue
            }
        }
    }

    return deviceIdString;
}

func IsDeviceConnected(deviceIdString string) bool {
    return (gPigeon.Mailbox(deviceIdString) != nil)
}

// Main websocket server routine.
// This event loop runs until the websocket connection is broken.
func CanopyWebsocketServer(ws *websocket.Conn) {

    var mailbox *pigeon.PigeonMailbox
    var cnt int32
    
    cnt = 0

    // connect to cassandra
    dl := cassandra_datalayer.NewDatalayer()
    conn, err := dl.Connect("canopy")
    if err != nil {
        fmt.Println("Could not connect to database")
        return
    }
    defer conn.Close()

    for {
        var in string

        // check for message from client
        ws.SetReadDeadline(time.Now().Add(100*time.Millisecond))
        err := websocket.Message.Receive(ws, &in)
        if err == nil {
            // success, payload received
            cnt++;
            deviceId := processPayload(conn, in, cnt)
            if deviceId != "" && mailbox == nil {
                mailbox = gPigeon.CreateMailbox(deviceId)
            }
        } else if err == io.EOF {
            // connection closed
            if mailbox != nil {
                mailbox.Close()
            }
            return;
        } else if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
            // timeout reached, no data for me this time
        } else {
            fmt.Println("Unexpected error:", err);
        }

        if mailbox != nil {
            msg, _ := mailbox.RecieveMessage(time.Duration(100*time.Millisecond))
            if msg != nil {
                msgString, err := json.Marshal(msg)
                if err != nil {
                    fmt.Println("Unexpected error:", err);
                }
                
                websocket.Message.Send(ws, msgString)
            }
        }
    }
}
