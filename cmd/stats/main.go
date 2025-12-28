/*
Copyright 2025.

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
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	stats_shared "github.com/willmurnane/clusterfuzz/internal/stats"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	fuzzerStatPath = "/data/*/fuzzer_stats"
)

// nolint:gocyclo
func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	client, clientErr := createClient()
	if clientErr != nil {
		fmt.Printf("Error creating kubernetes client: %v\n", clientErr)
		return
	}
	ctx := context.Background()
	// TODO make time configurable via env var
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		update_pod_annotations(ctx, client)
	}
}
func update_pod_annotations(ctx context.Context, client kubernetes.Interface) {
	var podName = os.Getenv("HOSTNAME")
	var podNamespace = os.Getenv("POD_NAMESPACE")

	annotations := make(map[string]string)
	annotations[stats_shared.AnnotationPrefix+"stats-timestamp"] = time.Now().Format(time.RFC3339)
	// TODO: read /data/*/fuzzer_stats, aggregate results, annotate the pod.
	statsFiles, globErr := filepath.Glob(fuzzerStatPath)
	if globErr != nil {
		fmt.Printf("Error globbing for stats files in %s: %v\n", fuzzerStatPath, globErr)
		return
	}
	stats := make(map[string][]string)
	for _, statsPath := range statsFiles {
		file, fileErr := os.Open(statsPath)
		if fileErr != nil {
			fmt.Printf("Error opening stats file %s: %v\n", statsPath, fileErr)
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) != 2 {
				slog.Warn("Invalid stats line", "file", statsPath, "line", line)
				continue
			}
			key := strings.TrimSpace(parts[0])
			stats[key] = append(stats[key], strings.TrimSpace(parts[1]))
		}
		closeErr := file.Close()
		if closeErr != nil {
			fmt.Printf("Error closing stats file %s: %v\n", statsPath, closeErr)
		}
	}
	for key, updates := range stats {
		aggregatedValue := stats_shared.Reduce(key, updates)
		annotations[stats_shared.AnnotationPrefix+key] = aggregatedValue
	}
	slog.Debug("Updating pod annotations",
		slog.String("pod", fmt.Sprintf("%s/%s", podNamespace, podName)),
		slog.Int("stats_files", len(statsFiles)),
		slog.Int("count", len(annotations)))
	patchBytes, serializeErr := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": annotations,
		},
	})
	if serializeErr != nil {
		fmt.Printf("Error serializing annotation patch for pod %s/%s: %v\n", podNamespace, podName, serializeErr)
		return
	}

	_, podUpdateErr := client.CoreV1().Pods(podNamespace).
		Patch(ctx, podName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if podUpdateErr != nil {
		fmt.Printf("Error updating pod %s/%s: %v\n", podNamespace, podName, podUpdateErr)
		return
	}
}

func createClient() (kubernetes.Interface, error) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig",
			filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	clientConfig, inClusterErr := rest.InClusterConfig()
	if inClusterErr != nil {
		fromPathConfig, fromPathErr := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if fromPathErr != nil {
			return nil, fmt.Errorf("unable to load kubeconfig from %s: %v or as in-cluster: %v",
				*kubeconfig, fromPathErr, inClusterErr)
		} else {
			clientConfig = fromPathConfig
		}
	}

	client, clientErr := kubernetes.NewForConfig(clientConfig)
	if clientErr != nil {
		return nil, fmt.Errorf("unable to create a client: %v", clientErr)
	}

	return client, nil
}
