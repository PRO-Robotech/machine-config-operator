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

// Package main is the entry point for the MCO Agent.
// The agent runs on each node and applies machine configuration.
package main

import (
	"flag"
	"os"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/agent"
	mcoclient "in-cloud.io/machine-config/pkg/client"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var nodeName string
	var hostRoot string
	var skipSystemd bool
	var noReboot bool
	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "Name of the node this agent runs on")
	flag.StringVar(&hostRoot, "host-root", "/host", "Path prefix for host filesystem")
	flag.BoolVar(&skipSystemd, "skip-systemd", false, "Skip systemd (for envs without systemd)")
	flag.BoolVar(&noReboot, "no-reboot", false, "Disable actual reboots (use NoOpExecutor for testing)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if nodeName == "" {
		setupLog.Error(nil, "node-name is required (set NODE_NAME env or --node-name flag)")
		os.Exit(1)
	}

	setupLog.Info("starting mco-agent", "node", nodeName, "hostRoot", hostRoot)

	config := ctrl.GetConfigOrDie()
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes client")
		os.Exit(1)
	}

	scheme := mcov1alpha1.AddToScheme
	rtClient, err := client.New(config, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to create controller-runtime client")
		os.Exit(1)
	}

	if err := scheme(rtClient.Scheme()); err != nil {
		setupLog.Error(err, "unable to add MCO scheme")
		os.Exit(1)
	}

	mcoClient := mcoclient.NewRuntimeClient(rtClient)
	ctx := ctrl.SetupSignalHandler()

	// Use no-op systemd connection if skipSystemd is set
	var systemdConn agent.SystemdConnection
	if skipSystemd {
		systemdConn = agent.NewNoOpSystemdConnection()
	}

	agentInstance, err := agent.NewWithContext(ctx, agent.Config{
		NodeName:    nodeName,
		K8sClient:   k8sClient,
		MCOClient:   mcoClient,
		HostRoot:    hostRoot,
		SystemdConn: systemdConn,
		NoReboot:    noReboot,
	})
	if err != nil {
		setupLog.Error(err, "unable to create agent")
		os.Exit(1)
	}
	defer agentInstance.Close()

	setupLog.Info("agent initialized, starting main loop")
	if err := agentInstance.Run(ctx); err != nil {
		setupLog.Error(err, "agent failed")
		os.Exit(1)
	}

	setupLog.Info("agent shutdown complete")
}
