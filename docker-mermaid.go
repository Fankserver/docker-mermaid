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
)

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

	networks, err := cli.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, network := range networks {
		text += fmt.Sprintf(
			"network_%s{%s}\n",
			network.ID,
			network.Name,
		)
	}

	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}


	for _, service := range services {
		text += fmt.Sprintf(
			"service_%s(%s)\n",
			service.ID,
			service.Spec.Name,
		)

		for _, network := range service.Spec.Networks {
			text += fmt.Sprintf(
				"service_%s --- network_%s\n",
				service.ID,
				network.Target,
			)
		}
	}

	fmt.Fprint(w, text)
}
