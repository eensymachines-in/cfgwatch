# CfgWatch.service
---
Systemd service running on the device that maintains a subscriber link to the upstream [webapi-devicereg](https://github.com/eensymachines-in/webapi-devicereg) server for the commands to receive and cross checking the __device registration__. When no registration is identified on the server, the device shall go ahead to register itself. Registration information of the device is found combined from 2 files:

1. aquapone.config.json
2. auapone.reg.json

Subsequnetly after this one time registration the device can then just get 200OK from the server as an indication that the device is registered. 
Upon failure to register or error getting the registration the service shall panic fatally and will be aborted. - __Currently we make only one attempt to establish link to the upstream server and verify the registartion.__

On receiving a json command from RabbitMQ this service shall overwrite the config.json file and restart the main `aquapone.service`
```
        rabbit--amqp    webapi-devicereg <--| 
        (json command)  (device registration)
                v           |               |
                |        HTTP GET       HTTP POST
                |           v               |
        cfgwatch.service----|-----------|---|
                |        read         read
                |---------|             |
                write     |             |        
        |-------|         ^             ^
        |       v         |             |
        |    aquapone.config.json    aquapone.reg.json  
    restart
        |
    aquapone.service
```     

### Setting up  
---- 

The application necessitates the presence of two key configuration or registration files:

- Schedule Configuration: This file serves as a repository for the schedule configuration utilized by the aquaponics system, facilitating the management of scheduling parameters.
- Device Configuration: This file contains essential registration data, required for the initialization and ongoing operation of the system.
     

```sh
ssh niranjan@rpi0w.local
# here we make 2 soft links and these are from repository onto the 
sudo ln -s /home/niranjan/source/github.com/eensymachines-in/aquapone/aquapone.config.json /etc/aquapone.config.json
sudo ln -s /home/niranjan/source/github.com/eensymachines-in/cfgwatch/aquapone.reg.json /etc/aquapone.reg.json
```
While application code files and the above configurations reside in local directories there has to be a standard directory from which services (systemctl units) shall be able to access the same. Hence we prefer to create some __soft links__ to the files.

```sh 
ls -l /etc | grep -sw 'aquapone.*.json'
```
You would find such soft links that point back to 2 files on the source code directory
```
lrwxrwxrwx  1 root root      79 Mar 23 10:40 aquapone.config.json -> /home/niranjan/source/github.com/eensymachines-in/aquapone/aquapone.config.json
lrwxrwxrwx  1 root root      76 Apr 21 11:31 aquapone.reg.json -> /home/niranjan/source/github.com/eensymachines-in/cfgwatch/aquapone.reg.json
```
Peep inside the registration file, device details excluding the schedule configurations 

```json
{ 
    "mac":  "**:**:**:**:**",
    "name": "Aquaponics pump control-I, Saidham",
    "make": "Raspberry Pi, 0w 512M 16G",
    "users": ["johndoe@gmail.com"],
    "location": "18.417440, 73.769136"
}
```
Inside the configuration file, is the schedule without any registration information.

```json
{"appname":"","schedule":{"config":1,"tickat":"13:00","pulsegap":20,"interval":35}}
```

The application conducts a verification process on the registration upstream server using the device's MAC ID. Subsequently, it merges the retrieved schedule data with the offline registration information, preparing it for transmission to the server as a new registration entry. Notably, the registration process on the server encompasses both registration and configuration aspects. However, on the device, these components are maintained in separate files, a design choice aimed at facilitating streamlined access to distinct data sets at various stages of operation.

#### Making the `aquapone.reg.json`
----

```json
{ 
    "mac":  "**:**:**:**:**",
    "name": "Aquaponics pump control-I, Saidham",
    "make": "Raspberry Pi, 0w 512M 16G",
    "users": ["kneerunjun@gmail.com"],
    "location": "18.417440, 73.769136"
}
```

The MAC ID is something that needs to be extracted from the device, while other fields can be contextually entered.

```sh
ifconfig | grep ether

```

```
ether <macid:you:were:seeking>  txqueuelen 1000  (Ethernet)
```
Use this mac id in the json configuration

#### Environment vars

```sh
cat ~/.bashrc
```

``` sh
#for eensymachines/cfgwatch application
export PATH_APPCONFIG=/etc/aquapone.config.json
export PATH_APPREG=/etc/aquapone.reg.json
export UPSTREAM_URL=http://aqua.eensymachines.in:30001/api/devices
export NAME_SYSCTLSERVICE=aquapone.service
export MODE_SYSCTLCMD=0 #0=restart 1=stop 2=start
export MODE_DEBUGLVL=5

#for amqp rabbit connections
#changes to this will affect all the applications accessing rabbitmq
export AMQP_LOGIN=****** # login for rabbit server
export AMQP_SERVER=aqua.eensymachines.in:30567
export AMQP_QUE=config_alerts
export AMQP_XCHG=configs_direct

#for the aquapone application 
export GPIO_TOUCH=31
export GPIO_ERRLED=33
export GPIO_PUMP_MAIN=35
```
`bashrc` shall define variables that are required by `cfgwatch` and `aquapone`

Make sure the environment variables defined in service unit as well. Systemctl _DOES NOT_ use bashrc env files. Service shall then find the environment vars in place when it runs

```
[Service]
Type=simple
Environment="PATH_APPCONFIG=/etc/aquapone.config.json" 
Environment="PATH_APPREG=/etc/aquapone.reg.json" 
Environment="NAME_SYSCTLSERVICE=aquapone.service" 
Environment="MODE_SYSCTLCMD=0" 
Environment="MODE_DEBUGLVL=5" 
Environment="AMQP_SERVER=65.20.79.167:5672" 
Environment="AMQP_LOGIN=**********" 
Environment="AMQP_CFGCHNNL=config-alerts" 
ExecStart=/usr/bin/cfgwatch


```
A streamlined negotiation loop coded in GoLang, designed to monitor RabbitMQ for message triggers signaling changes in configuration. This system can dynamically adjust configurations and initiate restarts for the specified services as needed.

#### Build and get going:
-----

To build the Go Application and setup the systemctl services 

```sh 
# Without sudo 
./build.sh 
```

```sh 
systemctl status cfgwatch.service
```
```sh
● cfgwatch.service - Starts the configuration watch service on the device
     Loaded: loaded (/etc/systemd/system/cfgwatch.service; enabled; preset: enabled)
     Active: active (running) since Tue 2024-04-23 09:02:40 IST; 3h 34min ago
   Main PID: 1678 (cfgwatch)
      Tasks: 7 (limit: 377)
        CPU: 3.047s
     CGroup: /system.slice/cfgwatch.service
             └─1678 /usr/bin/cfgwatch

Apr 23 09:02:42 rpi0w cfgwatch[1678]: time="2024-04-23T09:02:42+05:30" level=debug ms>
Apr 23 09:02:42 rpi0w cfgwatch[1678]: time="2024-04-23T09:02:42+05:30" level=debug ms>
Apr 23 09:02:42 rpi0w cfgwatch[1678]: time="2024-04-23T09:02:42+05:30" level=debug ms>
Apr 23 09:02:42 rpi0w cfgwatch[1678]: time="2024-04-23T09:02:42+05:30" level=info msg>
Apr 23 09:13:07 rpi0w cfgwatch[1678]: time="2024-04-23T09:13:07+05:30" level=debug ms>
Apr 23 09:13:07 rpi0w cfgwatch[1678]: time="2024-04-23T09:13:07+05:30" level=debug ms>
Apr 23 09:13:07 rpi0w sudo[1813]:     root : PWD=/ ; USER=root ; COMMAND=/usr/bin/sys>
Apr 23 09:13:07 rpi0w sudo[1813]: pam_unix(sudo:session): session opened for user roo>
Apr 23 09:13:07 rpi0w sudo[1813]: pam_unix(sudo:session): session closed for user root
Apr 23 09:13:07 rpi0w cfgwatch[1678]: time="2024-04-23T09:13:07+05:30" level=debug ms>
lines 1-19/19 (END)

```

As you can notice the `cfgwatch` service is up and running, send commands via the rabbit broker to change the schedule configuration.
To see the logs for the running service here is the command 

```sh
journalctl -u cfgwatch.service --since today
```
To check to see if schedule has been changed 

```sh
cat /etc/aquapone.config.json
```
