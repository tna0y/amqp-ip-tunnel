package tunnel

import "net"

type AMQPIPTunnelConfig struct {
	ExchangeName     string
	ConnectionURI    string
	InterfaceIPNet   *net.IPNet
	TUNInterfaceName string
	DebugPackets     bool
}

func (c *AMQPIPTunnelConfig) applyDefaults() {
	if c.ExchangeName == "" {
		c.ExchangeName = "router-ip"
	}
	if c.TUNInterfaceName == "" {
		c.TUNInterfaceName = "amqp-tun0"
	}
}
