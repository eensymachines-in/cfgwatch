# CfgWatch
Microservice running on the device that maintains a subscriber link to the upstream [webapi-devicereg]() server for the commands to receive and cross checking the device registration.

### Setting up  
---- 
Now that you have the repository what do you next ?

Application needs 2 configuration / registration files :
- Schedule configuration:
    Stores the schedule configuration used by the actual aquapone configuraion of the schedule 
- Device configuration
    this one time registration information i

```sh
ssh niranjan@rpi0w.local
# here we make 2 soft links and these are from repository onto the 
sudo ln -s /home/niranjan/source/github.com/eensymachines-in/aquapone/aquapone.config.json /etc/aquapone.config.json
sudo ln -s /home/niranjan/source/github.com/eensymachines-in/cfgwatch/aquapone.reg.json /etc/aquapone.reg.json
```
Applications do run as service and such require a distinct _standard_ diretory from which they can refer to files.
Now check to see if the files have been created.

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
    "mac":  "b8:27:eb:a5:be:48",
    "name": "Aquaponics pump control-I, Saidham",
    "make": "Raspberry Pi, 0w 512M 16G",
    "users": ["kneerunjun@gmail.com"],
    "location": "18.417440, 73.769136"
}

```
#### Whats schedule go to do with `cfgwatch` ?
---

`Cfgwatch` needs the schedule
- When registration isnt found on the upstream server
- When writing new schedule, upon change. - command from amqpo server.

Hence though counter intuitive `cfgwatch` does need access to `/etc/aquapone.config.json`


```
cat ~/.bashrc
```

```
#for eensymachines/cfgwatch application
export PATH_APPCONFIG=/etc/aquapone.config.json
export PATH_APPREG=/etc/aquapone.reg.json
export NAME_SYSCTLSERVICE=aquapone.service
export MODE_SYSCTLCMD=0 #0=restart 1=stop 2=start
export MODE_DEBUGLVL=5

#for amqp rabbit connections
#changes to this will affect all the applications accessing rabbitmq
export AMQP_LOGIN=******* # login for rabbit server
export AMQP_SERVER=65.20.79.167:5672 # base url for rabbit server
export AMQP_CFGCHNNL=config-alerts #channel over which triggers for config changes

#for the aquapone application 
export GPIO_TOUCH=31
export GPIO_ERRLED=33
export GPIO_PUMP_MAIN=35

```

Make sure the environment variables defined in service unit as well. Systemctl does not use bashrcenv files. Service shall then find the environment vars in place when it runs

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

A simple negotiating loop written in GoLang which can watch RabbitMQ for message triggers for change in configuration and can change configuration and restart the desired services


