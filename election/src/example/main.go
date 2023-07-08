/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	election "lib"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/api"

	// "k8s.io/kubernetes/pkg/apimachinery/types"
	"k8s.io/kubernetes/pkg/client/restclient"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	kubectl_util "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	flags = flag.NewFlagSet(
		`elector --election=<name>`,
		flag.ExitOnError)
	name      = flags.String("election", "", "The name of the election")
	id        = flags.String("id", "", "The id of this participant")
	namespace = flags.String("election-namespace", api.NamespaceDefault, "The Kubernetes namespace for this election")
	ttl       = flags.Duration("ttl", 10*time.Second, "The TTL for this election")
	inCluster = flags.Bool("use-cluster-credentials", false, "Should this request use cluster credentials?")
	addr      = flags.String("http", "", "If non-empty, stand up a simple webserver that reports the leader state")

	leader = &LeaderData{}
)

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

func makeClient() (*client.Client, error) {
	var cfg *restclient.Config
	var err error

	if *inCluster {
		if cfg, err = restclient.InClusterConfig(); err != nil {
			return nil, err
		}
	} else {
		clientConfig := kubectl_util.DefaultClientConfig(flags)
		if cfg, err = clientConfig.ClientConfig(); err != nil {
			return nil, err
		}
	}
	return client.New(cfg)
}

// LeaderData represents information about the current leader
type LeaderData struct {
	Name string `json:"name"`
}

func webHandler(res http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(leader)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

func webHealthHandler(res http.ResponseWriter, _ *http.Request) {
	if leader == nil || leader.Name == "" {
		res.WriteHeader(http.StatusInternalServerError)
		io.WriteString(res, fmt.Sprintf("Invalid leader set: %v", leader))
		return
	}

	res.WriteHeader(http.StatusOK)
	io.WriteString(res, fmt.Sprintf("Valid leader set: %v", leader))
}

func webLeaderHandler(res http.ResponseWriter, _ *http.Request) {
	if leader == nil || leader.Name == "" {
		res.WriteHeader(http.StatusInternalServerError)
		io.WriteString(res, fmt.Sprintf("Invalid leader set: %v", leader))
		return
	}
	if leader.Name == *id {
		res.WriteHeader(http.StatusOK)
		io.WriteString(res, fmt.Sprintf("Valid leader set: %v", leader))
		return
	}
	res.WriteHeader(http.StatusGone)
}

func validateFlags() {
	if len(*id) == 0 {
		*id = os.Getenv("HOSTNAME")
		if len(*id) == 0 {
			glog.Fatal("--id cannot be empty")
		}
	}
	if len(*name) == 0 {
		glog.Fatal("--election cannot be empty")
	}
}

func main() {
	flags.Parse(os.Args)
	validateFlags()

	kubeClient, err := makeClient()
	if err != nil {
		glog.Fatalf("error connecting to the client: %v", err)
	}

	fn := func(str string) {
		var payload []patchStringValue

		if *id == str {
			payload = []patchStringValue{{
				Op:    "add",
				Path:  "/metadata/labels/leader",
				Value: "yes",
			}}
		} else {
			payload = []patchStringValue{{
				Op:   "remove",
				Path: "/metadata/labels/leader",
			}}
		}
		// var updateErr error
		payloadBytes, _ := json.Marshal(payload)
		_, updateErr := kubeClient.Pods(*namespace).Patch(*id, "application/json-patch+json", payloadBytes)
		if updateErr == nil {
			fmt.Println(fmt.Sprintf("Pod %s labelled successfully.", *id))
		} else {
			fmt.Println(updateErr)
		}

		leader.Name = str
		fmt.Printf("%s is the leader\n", leader.Name)
	}

	e, err := election.NewElection(*name, *id, *namespace, *ttl, fn, kubeClient)
	if err != nil {
		glog.Fatalf("failed to create election: %v", err)
	}
	go election.RunElection(e)

	if len(*addr) > 0 {
		http.HandleFunc("/health", webHealthHandler)

		http.HandleFunc("/leader", webLeaderHandler)
		http.HandleFunc("/", webHandler)
		http.ListenAndServe(*addr, nil)
	} else {
		select {}
	}
}
