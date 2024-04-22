package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	AMQP_LOGIN          = "guest:guest"               // rabbitmq credentials
	AMQP_SERVER         = "192.168.1.102:30073"       // rabbitmq base url location
	CONFIG_PATH         = "/etc/aquapone.config.json" // location of the configuration to write
	RESTART_SERVICE     = "aquapone.service"          // name of the service
	SYSCTL_CMD          = RESTART                     // after the configuration is applied
	RETRIES_BEFORE_FAIL = 1                           // tries to establish amqp connection
	SLEEP_BEFORE_RETRY  = 5 * time.Second
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
		"PATH_APPREG",
		"UPSTREAM_URL",
		"NAME_SYSCTLSERVICE",
		"MODE_SYSCTLCMD",
		"MODE_DEBUGLVL",
		"AMQP_SERVER",
		"AMQP_LOGIN",
		"AMQP_QUE",
		"AMQP_XCHG",
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

// jsonFToMap : reads in any json file, and sends out map as a resutl
func jsonFToMap(path string) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	byt, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(byt, &result)
	if err != nil {
		return nil, err
	}
	return result, nil

}

// listenOnRabbitQ : sets up the listening queue on the exchange aas specified
// NOTE: all the environment variables required to connect successfully
// returns a channel over which messages can be subscribed to
// cancel function that lets you clos econnection
// error incase connecting with amqp gateway
// routing key is the mac id of the device - hence this takes in the mac id
func listenOnRabbitQ(macid string) (<-chan amqp.Delivery, func(), error) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s@%s/", os.Getenv("AMQP_LOGIN"), os.Getenv("AMQP_SERVER")))
	if err != nil || conn == nil {
		return nil, nil, fmt.Errorf("failed to connect to AMQP server %s", err)
	}
	closeConn := func() {
		log.Warn("Now closing rabbit connection..")
		conn.Close()
	}
	log.WithFields(log.Fields{
		"connect_notnil": conn != nil,
	}).Debug("amqp dial..")
	ch, err := conn.Channel()
	if err != nil || ch == nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("failed to open any rabbit channel")
		return nil, closeConn, fmt.Errorf("unable to get channel from connection %s", err)
	}
	q, err := ch.QueueDeclare(
		os.Getenv("AMQP_QUE"), //Qname
		false,                 // durable
		false,                 //auto delete
		false,                 //exclusive
		false,                 // nowait
		nil,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"err":    err,
			"q_name": os.Getenv("AMQP_QUE"),
		}).Error("failed q declaration..")
		return nil, closeConn, fmt.Errorf("failed to declare queue  %s", err)
	}
	log.WithFields(log.Fields{
		"q_name": q.Name,
	}).Debug("queue declared..")
	err = ch.QueueBind(q.Name, macid, os.Getenv("AMQP_XCHG"), false, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"err":    err,
			"q_name": os.Getenv("AMQP_QUE"),
		}).Error("failed q binding..")
		return nil, closeConn, fmt.Errorf("exchange binding failed %s", err)
	}
	log.WithFields(log.Fields{
		"x_name": os.Getenv("AMQP_XCHG"),
	}).Debug("queue bound to exchange..")
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
		log.WithFields(log.Fields{
			"err": err,
		}).Error("failed start consuming..")
		return nil, closeConn, fmt.Errorf("failed to setup consuming channel on amqp queue %s", err)
	}
	log.Debug("start consuming messagess..")
	log.WithFields(log.Fields{
		"mac":      macid,                  //topic for routing
		"q_name":   q.Name,                 // name of the queue
		"exchange": os.Getenv("AMQP_XCHG"), // name of the excahnge
	}).Info("Now listening on rabbit connection ..")
	return msgs, closeConn, nil
}

// checkRegOrRegister : Checks for the registry of the device
// If device is already registered will proceed ok with no action
// Incase the device isnt registered will register and then proceed ok
// Incase registry isnt able to confirm or the device isnt able to register itself then error
// incase of error the service will fall apart
// sends back the registration object as well
func checkRegOrRegister() error {
	regis, err := jsonFToMap(os.Getenv("PATH_APPREG"))
	if err != nil {
		return err
	}
	config, err := jsonFToMap(os.Getenv("PATH_APPCONFIG"))
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"reg_is_nil": regis == nil,
		"cfg_is_nil": config == nil,
	}).Debug("failed to read configuration files..")
	regis["cfg"] = config["schedule"]
	log.WithFields(log.Fields{
		"regis": regis,
	}).Debug("merging registration and configuration")

	url := fmt.Sprintf("%s/%s", os.Getenv("UPSTREAM_URL"), regis["mac"])
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	cl := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"url": os.Getenv("UPSTREAM_URL"),
	}).Debug("queried server for existing configuration")

	if resp.StatusCode == http.StatusNotFound {
		// time to register device with the server
		log.Debug("registration not found..")
		url = os.Getenv("UPSTREAM_URL")
		byt, err := json.Marshal(regis)
		if err != nil {
			return err
		}
		req, err = http.NewRequest("POST", url, bytes.NewReader(byt))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err = cl.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error registering device %d", resp.StatusCode)
		} else {
			log.Info("registered new device..")
		}
	} else if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		byt, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Warn("failed to read registration")
		}
		result := map[string]interface{}{}
		json.Unmarshal(byt, &result)

		log.WithFields(log.Fields{
			"mac":  result["mac"],
			"name": result["name"],
			"make": result["make"],
		}).Info("existing registration found")
		return nil
	} else {
		return fmt.Errorf("error getting device registration %d", resp.StatusCode)
	}
	return nil
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
	/* Check for device registration upstream
	Incase the device isnt registered it would then register itself first */
	if err := checkRegOrRegister(); err != nil {
		log.Fatalf("Failed to check for device registry upstream %s", err)
		return
	}
	regis, err := jsonFToMap(os.Getenv("PATH_APPREG"))
	if err != nil {
		log.Fatal("unable to read device registration")
	}
	msgs, closeConn, err := listenOnRabbitQ(regis["mac"].(string))
	if err != nil {
		log.Fatal(err)
	}
	defer closeConn()

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
				err := json.Unmarshal(d.Body, &cfg.Schedule) //reading config from messages
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
