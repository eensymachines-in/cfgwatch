package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/eensymachines-in/patio/aquacfg"
	"github.com/eensymachines-in/patio/interrupt"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

/*===========
Applications often are pivoted on configurations & configurations are bound to change which should trigger restarting the application services and thus re-read of the configuration. Change of the configurations often are initated remotely online to have IoT like capabilities.
Here we have a microservice that runs alongside the main application, and watches the Message queue remotely for any commands or change in configurations. Upon getting a simple trigger this can restart the desired service

author		: kneerunjun@gmail.com
date		: 13-3-2024
place		: Pune

===============
*/

// Systemctl units have distinct actions that you can call on trigger
type ServiceAction uint8

const (
	RESTART ServiceAction = iota
	STOP
	START
)

var (
	AMQP_LOGIN      = "guest:guest"               // rabbitmq credentials
	AMQP_SERVER     = "192.168.1.102:30073"       // rabbitmq base url location
	CONFIG_PATH     = "/etc/aquapone.config.json" // location of the configuration to write
	RESTART_SERVICE = "aquapone.service"          // name of the service
	SYSCTL_CMD      = RESTART                     // after the configuration is applied
)

func init() {
	// required environment variables
	/*
		PATH_APPCONFIG
		NAME_SYSCTLSERVICE
		MODE_SYSCTLCMD
		MODE_DEBUGLVL
		AMQP_SERVER
		AMQP_LOGIN
		AMQP_CFGCHNNL
	*/
	for _, v := range []string{
		"PATH_APPCONFIG",
		"NAME_SYSCTLSERVICE",
		"MODE_SYSCTLCMD",
		"MODE_DEBUGLVL",
		"AMQP_SERVER",
		"AMQP_LOGIN",
		"AMQP_CFGCHNNL",
	} {
		if val := os.Getenv(v); val == "" {
			log.Panicf("Required environment variable missing in ~/.bashrc: %s", v)
		}
	}

	// Setting up the logging framework
	log.SetFormatter(&log.TextFormatter{DisableColors: false, FullTimestamp: false})
	log.SetReportCaller(false)
	log.SetOutput(os.Stdout)
	lvl, err := strconv.Atoi(os.Getenv("MODE_DEBUGLVL"))
	if err != nil {
		log.Warnf("invalid env var value for logging level, only integers %s", os.Getenv("MODE_DEBUGLVL"))
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.Level(lvl)) // sets from the environment
	}

}

func main() {
	log.Info("Now starting config watcher application")
	defer log.Warn("Closing config watcher application")
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for intr := range interrupt.SysSignalWatch(ctx, &wg) {
			log.WithFields(log.Fields{
				"time": intr.Format(time.RFC822),
			}).Warn("Interrupted...")
			cancel() // time for all the program to go down
		}
	}()

	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s@%s/", os.Getenv("AMQP_LOGIN"), os.Getenv("AMQP_SERVER")))
	if err != nil {
		log.Panicf("Failed to connect to Rabbit server %s", err)
		return
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		log.Panicf("Failed to initiate channel on Rabbit server %s", err)
		return
	}
	q, err := ch.QueueDeclare(
		os.Getenv("AMQP_CFGCHNNL"), // name
		false,                      // durable
		false,                      // delete when unused
		false,                      // exclusive
		false,                      // no-wait
		nil,                        // arguments
	)
	if err != nil {
		log.Panicf("Failed to declare queue on Rabbit server %s", err)
		return
	}
	log.Info("connected to RabbitMQ...")
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Panicf("Failed to setup consuming channel on rabbit server %s", err)
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				log.Warn("Now halting watch from Rabbit..")
				return
			case d := <-msgs:
				log.Debug("Received command..")
				cfg := aquacfg.AppConfig{}
				// NOTE: cannot send in partial configuration, even if unchanged the entire configuration has to be shuttled to-fro for the changes to be applied correctly
				err := json.Unmarshal(d.Body, &cfg) //reading config from messages
				if err != nil {
					log.Errorf("Failed to read command message from Rabbit %s", err)
					continue
				}
				byt, _ := json.Marshal(&cfg)
				if err := os.WriteFile(os.Getenv("PATH_APPCONFIG"), byt, os.ModePerm); err != nil { //writing config to file
					log.Errorf("failed to write configuration: %s", err)
					continue
				}
				log.Debug("New configuration applied..")
				/*
					Service action after the configuration is applied
					This is configurable from command line arguments
				*/
				execCmd := func(sa ServiceAction) error {
					args := []string{
						"-c",
					}
					if sa == RESTART { // service restarts
						args = append(args, fmt.Sprintf("sudo systemctl restart %s", os.Getenv("NAME_SYSCTLSERVICE")))
					} else if sa == STOP { // service stops
						args = append(args, fmt.Sprintf("sudo systemctl stop %s", os.Getenv("NAME_SYSCTLSERVICE")))
					} else if sa == START { // service shall start
						args = append(args, fmt.Sprintf("sudo systemctl start %s", os.Getenv("NAME_SYSCTLSERVICE")))
					} else {
						return fmt.Errorf("unrecognised systemctl command")
					}
					cmd := exec.Command("/usr/bin/sh", args...)
					stdout, err := cmd.Output()
					if err != nil {
						return fmt.Errorf("failed to restart aquapone.service: %s", err)

					}
					log.Debug(stdout) // output of stadout for debug
					return nil
				}
				mode, err := strconv.Atoi(os.Getenv("MODE_SYSCTLCMD")) // from environment - restart/start/
				if err != nil {                                        //invalid mode #0=restart 1=stop 2=start
					log.Errorf("invalid mode for systemctl service action %s", os.Getenv("MODE_SYSCTLCMD"))
					continue
				}
				if err := execCmd(ServiceAction(mode)); err != nil { // proceed to systemctl command
					log.Error(err)
				}
			}
		}
	}()
	wg.Wait()
}
