package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tna0y/amqp-ip-tunnel/tunnel"
)

type CmdLineOpts struct {
	tunIP        string
	tunName      string
	amqpURI      string
	exchangeName string
	extraNets    string
	debug        bool
}

var (
	opts  CmdLineOpts
	flags = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
)

func init() {
	flags.StringVar(&opts.tunIP, "tun-ip", "", "IP address with a network mask to use on the TUN interface")
	flags.StringVar(&opts.tunName, "tun-iface-name", "amqp-tun0", "TUN interface name to use or create")
	flags.StringVar(&opts.amqpURI, "amqp-uri", "", "URI to use for connection to AMQP broker")
	flags.StringVar(&opts.exchangeName, "exchange", "ip-router", "AMQP Exchange to use or create")
	flags.StringVar(&opts.extraNets, "networks", "", "comma separated list of networks to route to this node. e.g. 1.0.0.0/24,2.0.0.0/16")
	flags.BoolVar(&opts.debug, "debug", false, "print all packets passing through")

	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Fatalln("Can't parse flags", err)
	}
}

func main() {
	var (
		recvNets []*net.IPNet
		ifaceNet *net.IPNet
		err      error
	)
	if opts.tunIP != "" {
		tunIP, tunNet, err := net.ParseCIDR(opts.tunIP)
		if err != nil {
			log.Fatalf("failed to parse IP addr \"%s\": %s", opts.tunIP, err.Error())
		}
		ifaceNet = &net.IPNet{IP: tunIP, Mask: tunNet.Mask}
		recvNets = append(recvNets, &net.IPNet{IP: tunIP, Mask: net.CIDRMask(32, 32)})
	}
	if opts.extraNets != "" {
		extraNetsStr := strings.Split(opts.extraNets, ",")
		for _, extraNetStr := range extraNetsStr {
			_, extraNet, err := net.ParseCIDR(extraNetStr)
			if err != nil {
				log.Fatalf("failed to parse IP net \"%s\": %s", extraNetStr, err.Error())
			}
			recvNets = append(recvNets, extraNet)
		}
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		cancelFn()
	}()

	config := tunnel.AMQPIPTunnelConfig{
		ExchangeName:     opts.exchangeName,
		ConnectionURI:    opts.amqpURI,
		InterfaceIPNet:   ifaceNet,
		TUNInterfaceName: opts.tunName,
		DebugPackets:     opts.debug,
	}

	tun, err := tunnel.NewAMQPIPTunnel(config)
	if err != nil {
		log.Fatalf("failed to setup AMQP Tunnel: %s", err.Error())
	}

	err = tun.Run(ctx)

	if err != nil {
		log.Fatalln("exited: ", err.Error())
	}
}
