package main

import (
	"context"
	"log"
	"net"

	"github.com/tna0y/amqp-ip-tunnel/tunnel"
)

func main() {
	config := tunnel.AMQPIPTunnelConfig{
		ConnectionURI: "amqp://guest:guest@RABBITMQ_HOST:5672/",
		DebugPackets:  true,
		InterfaceIPNet: &net.IPNet{
			IP:   net.ParseIP("10.1.0.1"),
			Mask: net.CIDRMask(24, 32),
		},
	}

	tun, err := tunnel.NewAMQPIPTunnel(config)
	if err != nil {
		log.Fatalf("failed to setup AMQP Tunnel: %s", err.Error())
	}

	err = tun.AddRoute(&net.IPNet{IP: net.ParseIP("10.2.0.0"), Mask: net.CIDRMask(16, 32)})
	if err != nil {
		log.Fatalf("failed to add route: %s", err.Error())
	}

	err = tun.Run(context.Background())

	if err != nil {
		log.Fatalln("exited: ", err.Error())
	}
}
