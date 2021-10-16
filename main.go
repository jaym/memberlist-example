package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-discover"
	"github.com/hashicorp/go-discover/provider/k8s"
	"github.com/hashicorp/memberlist"
)

type ChannelEventDelegate struct {
	Ch chan<- memberlist.NodeEvent
}

func (c *ChannelEventDelegate) NotifyJoin(n *memberlist.Node) {
	node := *n
	select {
	case c.Ch <- memberlist.NodeEvent{memberlist.NodeJoin, &node}:
	default:
	}
}

func (c *ChannelEventDelegate) NotifyLeave(n *memberlist.Node) {
	node := *n
	select {
	case c.Ch <- memberlist.NodeEvent{memberlist.NodeLeave, &node}:
	default:
	}
}

func (c *ChannelEventDelegate) NotifyUpdate(n *memberlist.Node) {
	node := *n
	select {
	case c.Ch <- memberlist.NodeEvent{memberlist.NodeUpdate, &node}:
	default:
	}
}

func main() {
	var nodes []string
	l := log.Default()

	var d discover.Discover
	discoverCfg := os.Getenv("DISCOVER_CFG")
	if discoverCfg != "" {
		d = discover.Discover{
			Providers: map[string]discover.Provider{
				"k8s": &k8s.Provider{},
			},
		}
		addrs, err := d.Addrs(discoverCfg, l)
		if err != nil {
			panic(err)
		}
		nodes = addrs
	} else {
		nodesEnv := os.Getenv("NODES")

		if nodesEnv != "" {
			nodes = strings.Split(nodesEnv, ",")
		}
	}

	config := memberlist.DefaultLANConfig()
	ch := make(chan memberlist.NodeEvent)
	config.Events = &ChannelEventDelegate{
		Ch: ch,
	}

	portStr := os.Getenv("PORT")
	if portStr != "" {
		port, err := strconv.ParseInt(portStr, 10, 16)
		if err != nil {
			panic(err)
		}
		config.BindPort = int(port)
	}
	config.Name = fmt.Sprintf("%s-%d", config.Name, config.BindPort)

	fmt.Println("Creating memberlist >")
	list, err := memberlist.Create(config)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Trying to join: %v\n", nodes)
	count, err := list.Join(nodes)
	if err != nil {
		panic(err)
	}

	fmt.Printf("joined %d nodes\n", count)

	stop := make(chan os.Signal, 1)

	// Register the signals we want to be notified, these 3 indicate exit
	// signals, similar to CTRL+C
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	goRoutineStopChan := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if discoverCfg == "" {
			return
		}
		ticker := time.NewTicker(30 * time.Second)
		for {
			select {
			case <-goRoutineStopChan:
				return
			case <-ticker.C:

				addrs, err := d.Addrs(discoverCfg, l)
				if err != nil {
					l.Printf("could not discover: %v", err)
				} else {
					members := list.Members()
					if len(members) == len(addrs) {
						l.Println("skipping join")
						continue
					}
					l.Println("trying join")
					if _, err := list.Join(addrs); err != nil {
						l.Printf("failed join: %v", err)
					}
				}
			}
		}
	}()

	timer := time.NewTicker(2 * time.Second)

LOOP:
	for {
		select {
		case <-timer.C:
			nodeStrs := []string{}
			for _, n := range list.Members() {
				nodeStrs = append(nodeStrs, fmt.Sprintf("node=%s state=%d", n.Name, n.State))
			}
			fmt.Printf("connected to: %v\n", strings.Join(nodeStrs, ","))
		case ev := <-ch:
			switch ev.Event {
			case memberlist.NodeJoin:
				fmt.Printf("node joined: %+v state=%d\n", ev.Node, ev.Node.State)
			case memberlist.NodeLeave:
				fmt.Printf("node left: %+v state=%d\n", ev.Node, ev.Node.State)
			case memberlist.NodeUpdate:
				fmt.Printf("node updated: %+v state=%d\n", ev.Node, ev.Node.State)
			}
		case <-stop:
			break LOOP
		}
	}

	close(goRoutineStopChan)
	wg.Wait()

	if err := list.Leave(time.Second * 5); err != nil {
		panic(err)
	}
}
