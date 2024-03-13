#!/usr/bin/sh 
# purpose is to build the go program, setup systemctl unit and start running
echo "Now building the configuration watch program..\n"

sudo go build -o /usr/bin/cfgwatch .  && sudo chmod 774 /usr/bin/cfgwatch

echo 'building systemctl unit..'
sudo systemctl enable $(pwd)/cfgwatch.service
sudo systemctl daemon-reload
sudo systemctl start cfgwatch.service
echo 'done..\nrun from /usr/bin/cfgwatch'
