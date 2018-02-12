package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"time"
	"net/http"
	"context"
	"github.com/rs/cors"
	"os"
	"os/signal"
	"github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"net"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/go-connections/nat"
	"github.com/docker/docker/api/types/container"
)

// dockerData holds the need data to the Provider p
type dockerData struct {
	ServiceName     string
	Name            string
	Labels          map[string]string // List of labels set to container or service
	NetworkSettings networkSettings
	Health          string
	Node            *types.ContainerNode
}

// NetworkSettings holds the networks data to the Provider p
type networkSettings struct {
	NetworkMode container.NetworkMode
	Ports       nat.PortMap
	Networks    map[string]*networkData
}

// Network holds the network data to the Provider p
type networkData struct {
	Name     string
	Addr     string
	Port     int
	Protocol string
	ID       string
}

func main() {
	// subscribe to SIGINT signals
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	http.HandleFunc("/generate", generateHandler)

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", "", 8080),
		Handler: cors.AllowAll().Handler(http.DefaultServeMux),
	}
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logrus.Fatalf("listen: %s\n", err)
		}
	}()

	<-quit
	logrus.Println("shutting down server...")
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctxTimeout); err != nil {
		logrus.Fatalf("could not shutdown: %v", err)
	}
	logrus.Println("server gracefully stopped")
}

func generateHandler(w http.ResponseWriter, r *http.Request) {
	cli, err := client.NewEnvClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	text := ""

	networkListArgs := filters.NewArgs()
	networkListArgs.Add("driver", "overlay")

	networkList, err := cli.NetworkList(context.Background(), types.NetworkListOptions{Filters: networkListArgs})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	networkMap := make(map[string]*types.NetworkResource)
	for _, network := range networkList {
		text += fmt.Sprintf(
			"network_%s{%s}\n",
			network.ID,
			network.Name,
		)

		networkToAdd := network
		networkMap[network.ID] = &networkToAdd
	}

	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}


	for _, service := range services {
		dData := parseService(service, networkMap)

		text += fmt.Sprintf(
			"service_%s(%s)\n",
			service.ID,
			service.Spec.Name,
		)

		for _, network := range dData.NetworkSettings.Networks {
			text += fmt.Sprintf(
				"service_%s --- network_%s\n",
				service.ID,
				network.ID,
			)
		}
	}

	fmt.Fprint(w, text)
}

func parseService(service swarm.Service, networkMap map[string]*types.NetworkResource) dockerData {
	dData := dockerData{
		ServiceName:     service.Spec.Annotations.Name,
		Name:            service.Spec.Annotations.Name,
		Labels:          service.Spec.Annotations.Labels,
		NetworkSettings: networkSettings{},
	}

	if service.Spec.EndpointSpec != nil {
		if service.Spec.EndpointSpec.Mode == swarm.ResolutionModeVIP {
			dData.NetworkSettings.Networks = make(map[string]*networkData)
			for _, virtualIP := range service.Endpoint.VirtualIPs {
				networkService := networkMap[virtualIP.NetworkID]
				if networkService != nil {
					ip, _, _ := net.ParseCIDR(virtualIP.Addr)
					network := &networkData{
						Name: networkService.Name,
						ID:   virtualIP.NetworkID,
						Addr: ip.String(),
					}
					dData.NetworkSettings.Networks[network.Name] = network
				} else {
					logrus.Debugf("Network not found, id: %s", virtualIP.NetworkID)
				}
			}
		}
	}
	return dData
}
