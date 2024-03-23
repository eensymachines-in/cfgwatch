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
