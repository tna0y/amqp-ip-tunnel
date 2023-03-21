package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vishvananda/netlink"
	"golang.org/x/sync/errgroup"
)

type AMQPIPTunnel struct {
	config AMQPIPTunnelConfig

	link      *netlink.Tuntap
	queueName string
	amqpChan  *amqp.Channel
}

func NewAMQPIPTunnel(config AMQPIPTunnelConfig) (*AMQPIPTunnel, error) {
	config.applyDefaults()
	tun := &AMQPIPTunnel{
		config: config,
	}

	err := tun.setupAMQP()
	if err != nil {
		return nil, err
	}
	err = tun.setupTUN()
	if err != nil {
		return nil, err
	}
	return tun, nil
}

func (t *AMQPIPTunnel) AddRoute(network *net.IPNet) error {
	routingKey, err := buildRoutingKey(network)
	if err != nil {
		return err
	}
	err = t.amqpChan.QueueBind(t.queueName, routingKey, t.config.ExchangeName, false, nil)
	if err != nil {
		return err
	}
	return nil
}

func (t *AMQPIPTunnel) RemoveRoute(network *net.IPNet) error {
	routingKey, err := buildRoutingKey(network)
	if err != nil {
		return err
	}
	err = t.amqpChan.QueueUnbind(t.queueName, routingKey, t.config.ExchangeName, nil)
	if err != nil {
		return err
	}
	return nil
}

func (t *AMQPIPTunnel) Run(ctx context.Context) error {
	wg, ctx := errgroup.WithContext(ctx)

	wg.Go(func() error {
		return t.sendPackets(ctx)
	})

	wg.Go(func() error {
		return t.recvPackets(ctx)
	})

	return wg.Wait()
}

func (t *AMQPIPTunnel) setupAMQP() error {
	conn, err := amqp.Dial(t.config.ConnectionURI)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		return err
	}

	err = ch.ExchangeDeclare(t.config.ExchangeName, "topic", true, false, false, false, nil)
	if err != nil {
		return err
	}

	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return err
	}

	t.amqpChan = ch
	t.queueName = queue.Name
	return nil
}

func (t *AMQPIPTunnel) setupTUN() error {
	link := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{
			Name: t.config.TUNInterfaceName,
		},
		Mode:   netlink.TUNTAP_MODE_TUN,
		Queues: 1,
	}

	err := netlink.LinkAdd(link)
	if err != nil {
		return err
	}
	err = netlink.LinkSetUp(link)
	if err != nil {
		return err
	}

	if t.config.InterfaceIPNet != nil {
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		addr := netlink.Addr{
			IPNet: t.config.InterfaceIPNet,
		}
		var found bool
		for _, exAddr := range addrs {
			if exAddr.Equal(addr) {
				found = true
				break
			}
		}
		if !found {
			err = netlink.AddrAdd(link, &addr)
			if err != nil {
				return err
			}
		}
	}
	t.link = link
	return nil
}

func (t *AMQPIPTunnel) sendPackets(ctx context.Context) error {
	var buf [2000]byte
	tunFd := t.link.Fds[0]
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := tunFd.Read(buf[:])
			if err != nil {
				return err
			}
			packet := gopacket.NewPacket(buf[:n], layers.LayerTypeIPv4, gopacket.NoCopy)
			ipLayer := packet.Layer(layers.LayerTypeIPv4)
			if ipLayer == nil {
				continue
			}
			ipPacket, _ := ipLayer.(*layers.IPv4)
			msg := amqp.Publishing{
				Body: buf[:n],
			}
			err = t.amqpChan.PublishWithContext(ctx, t.config.ExchangeName, ipPacket.DstIP.String(), false, false, msg)
			if err != nil {
				return fmt.Errorf("failed to publish message: %s", err.Error())
			}
			if t.config.DebugPackets {
				log.Printf("SEND: %s -> %s PROTO: %s LEN: %d\n", ipPacket.SrcIP.String(), ipPacket.DstIP.String(), ipPacket.Protocol.String(), n)
			}
		}
	}
}

func (t *AMQPIPTunnel) recvPackets(ctx context.Context) error {
	msgs, err := t.amqpChan.Consume(t.queueName, "", true, false, false, false, nil)

	tunFd := t.link.Fds[0]

	if err != nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-msgs:
			packet := gopacket.NewPacket(msg.Body, layers.LayerTypeIPv4, gopacket.NoCopy)
			ipLayer := packet.Layer(layers.LayerTypeIPv4)
			if ipLayer == nil {
				continue
			}
			ipPacket, _ := ipLayer.(*layers.IPv4)

			_, err = tunFd.Write(msg.Body)
			if err != nil {
				return fmt.Errorf("failed to write packet to TUN fd: %s", err.Error())
			}
			if t.config.DebugPackets {
				log.Printf("RECV: %s -> %s PROTO: %s LEN: %d\n", ipPacket.SrcIP.String(), ipPacket.DstIP.String(), ipPacket.Protocol.String(), len(msg.Body))
			}
		}
	}
}

func buildRoutingKey(network *net.IPNet) (string, error) {
	if len(network.IP) != net.IPv4len {
		return "", errors.New("only ipv4 networks are supported")
	}
	ones, _ := network.Mask.Size()
	if ones%8 != 0 {
		return "", errors.New("only subnets divisible by 8 are supported: /0, /8, /16, /24 and /32")
	}

	ipOctets := strings.Split(network.IP.String(), ".")
	ipOctets = ipOctets[:int(ones/8)]
	for i := 0; i < 4-int(ones/8); i++ {
		ipOctets = append(ipOctets, "*")
	}
	return strings.Join(ipOctets, "."), nil
}
