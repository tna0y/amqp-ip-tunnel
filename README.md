# Tunneling IP traffic trough AMQP

This repository contains used to route IP traffic through RabbitMQ. Despite being ineffective for production this method explores a possibility of using AMQP exchanges for IP packet routing.

# Installation
Binary release is available for Linux. You can download binaries attached to GitHub releases.

Package can also be installed as a golang package.
```go get github.com/tna0y/amqp-ip-tunnel```

# Usage

Running `amqp-ip-tun` requires root priveleges or `CAP_NET_ADMIN` capability.

Program is usable as standalone binary.

The following command will:
1. Create a TUN interface named `amqp-tun0` with `10.1.0.1/24` address assigned to it;
2. Send all packets received on `amqp-tun0` to the AMQP broker for routing;
3. Receive all packets addressed to `10.1.0.1/32` and `10.2.0.0/16` from the AMQP broker.

```bash
./amqp-ip-tun --tun-ip 10.1.0.1/24 --amqp-uri "amqp://guest:guest@$RABBITMQ_HOST:5672/" --networks 10.2.0.0/16 --debug 
```

Same result may be achieved when using the Golang package.

```golang
package main

import (
	"context"
	"log"
	"net"

	"github.com/tna0y/amqp-ip-tunnel/tunnel"
)

func main() {
	config := tunnel.AMQPIPTunnelConfig{
		ConnectionURI:  "amqp://guest:guest@RABBITMQ_HOST:5672/",
		DebugPackets:   true,
		InterfaceIPNet: &net.IPNet{
			IP: net.ParseIP("10.1.0.1"),
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

```