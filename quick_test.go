package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/eensymachines-in/patio/aquacfg"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
)

func TestAmqpConnection(t *testing.T) {
	msgs, closeConn, err := listenOnRabbitQ("b8:27:eb:a5:be:48")
	assert.Nil(t, err, "Unexpected error when connecting to rabbit")
	defer closeConn()
	for m := range msgs {
		t.Log(string(m.Body))
	}
}

func TestCheckReg(t *testing.T) {
	err := checkRegOrRegister()
	assert.Nil(t, err, "Unexpected error when check to register")
}

func TestGetDeviceReg(t *testing.T) {
	// TODO: check all the asserts statements, anc change errro meesages
	f, err := os.Open("/etc/aquapone.reg.json")
	assert.Nil(t, err, "Unexpected erorr when reading file contents")
	byt, err := io.ReadAll(f)
	assert.Nil(t, err, "Unexpected erorr when reading file contents")
	registration := map[string]interface{}{}
	err = json.Unmarshal(byt, &registration)
	assert.Nil(t, err, "Unexpected error when unmarshaling regitration")

	f, err = os.Open("/etc/aquapone.config.json")
	assert.Nil(t, err, "Unexpected erorr when reading file contents")
	byt, err = io.ReadAll(f)
	assert.Nil(t, err, "Unexpected erorr when reading file contents")
	schedule := map[string]interface{}{}
	err = json.Unmarshal(byt, &schedule)
	assert.Nil(t, err, "Unexpected error when unmarshaling regitration")

	registration["cfg"] = schedule["schedule"] // payload is ready for dispatch
	byt, err = json.Marshal(registration)
	assert.Nil(t, err, "Unexpected error when unmarshaling regitration")
	t.Log(string(byt))

}

func TestRestart(t *testing.T) {
	cmd := exec.Command("/usr/bin/sh", "-c", "sudo systemctl restart aquapone.service")
	// cmd := exec.Command("systemctl", "restart aquapone.service")
	stdout, err := cmd.Output()
	if err != nil {
		t.Errorf("failed to restart aquapone.service: %s %s", err, string(stdout))
		return
	}
	t.Log(string(stdout))
}

func TestSendRabbitMessage(t *testing.T) {
	conn, _ := amqp.Dial("amqp://guest:guest@192.168.1.102:30073/")
	ch, _ := conn.Channel()
	q, _ := ch.QueueDeclare(
		"aquapone.config", // name
		false,             // durable
		false,             // delete when unused
		false,             // exclusive
		false,             // no-wait
		nil,               // arguments
	)
	cfg := aquacfg.AppConfig{}
	f, _ := os.Open("/etc/aquapone.config.json")
	byt, _ := io.ReadAll(f)
	json.Unmarshal(byt, &cfg)
	// Modifying the existing configuration
	cfg.AppName = "Aquaponics control centrale"
	cfg.Schedule.TickAt = "13:40"
	cfg.Schedule.PulseGap = 3
	cfg.Schedule.Interval = 10
	cfg.Schedule.Config = 1

	body, _ := json.Marshal(&cfg)
	err := ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        body,
		})
	assert.Nil(t, err, "Unexpected error when testing send message to AMQP server")
}
