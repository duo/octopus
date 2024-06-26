package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/duo/octopus/internal/common"
	"github.com/duo/octopus/internal/master"
	"github.com/duo/octopus/internal/slave"

	log "github.com/sirupsen/logrus"
)

func main() {
	config, err := common.LoadConfig("configure.yaml")
	if err != nil {
		log.Fatal(err)
	}

	logLevel, err := log.ParseLevel(config.Log.Level)
	if err == nil {
		log.SetLevel(logLevel)
	}
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02 15:04:05", FullTimestamp: true})

	masterToSlave := common.NewMessageChan(1024)
	slaveToMaster := common.NewMessageChan(1024)

	master := master.NewMasterService(config, slaveToMaster.Out(), masterToSlave.In())
	master.Start()
	slave := slave.NewLimbService(config, masterToSlave.Out(), slaveToMaster.In())
	slave.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Printf("\n")

	slave.Stop()
	master.Stop()
}
