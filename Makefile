
all: pumpctrl

.PHONY: pumpctrl
pumpctrl:
	GOOS=linux GOARCH=arm GOARM=6 go build

.PHONY: upload
upload: pumpctrl
	ssh pi@192.168.1.55 "sudo systemctl stop pump"
	scp -r pumpctrl static pi@192.168.1.55:
	ssh pi@192.168.1.55 "sudo systemctl start pump"
