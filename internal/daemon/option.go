package daemon

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/axdx/raft-sandbox/internal/logging"
)

type NodeOpt func(*Node) error

func WithHostname(hostname string) NodeOpt {
	return func(n *Node) error {
		n.hostname = hostname
		return nil
	}
}

func WithPort(port string) NodeOpt {
	_, err := strconv.ParseUint(port, 10, 16)

	return func(n *Node) error {
		if err != nil {
			return err
		}
		n.port = port
		return nil
	}
}

func WithNodeList(potentialNodes []string) NodeOpt {
	nodes := make([]Connection, 0)
	errorNodes := make([]string, 0)

	for _, potential := range potentialNodes {
		if node, err := url.Parse(potential); err != nil {
			errorNodes = append(errorNodes, potential)
		} else {
			c := Connection{
				url:      node,
				lastSeen: nil,
			}
			nodes = append(nodes, c)
		}
	}

	logging.GetLogger().Printf("Nodes: %+v", nodes)

	return func(n *Node) error {
		if len(errorNodes) > 0 {
			return fmt.Errorf("error parsing node IDs: %s", strings.Join(errorNodes, ", "))
		}

		n.registry = nodes
		return nil
	}
}

func WithNodeID(id string) NodeOpt {
	return func(n *Node) error {
		n.id = id
		return nil
	}
}
