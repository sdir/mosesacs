package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sdir/mosesacs/cli"
	"github.com/sdir/mosesacs/daemon"
)

func main() {

	port := flag.Int("p", 9292, "Daemon port to listen on")
	flDaemon := flag.Bool("d", false, "Enable daemon mode")
	flVersion := flag.Bool("v", false, "Version")
	flHelp := flag.Bool("h", false, "Help")
	flUrl := flag.String("u", "localhost:9292", "Url to connect")
	flXmppUser := flag.String("xmpp-user", "", "Xmpp Username")
	flXmppPassword := flag.String("xmpp-pass", "", "Xmpp Password")
	flag.Parse()

	fmt.Printf("MosesACS %s by Luca Cervasio <luca.cervasio@gmail.com> (C)2014-2016 http://mosesacs.org\n", daemon.Version)

	if *flVersion {
		os.Exit(0)
	}

	if *flHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *flDaemon {
		logger := daemon.BasicWriter{}
		daemon.Run(port, &logger, *flXmppUser, *flXmppPassword)
	} else {
		client.Run(*flUrl)
	}
}
