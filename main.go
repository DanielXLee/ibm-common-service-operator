//
// Copyright 2020 IBM Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"flag"
	"os"

	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	operatorv3 "github.com/IBM/ibm-common-service-operator/api/v3"
	"github.com/IBM/ibm-common-service-operator/controllers"
	"github.com/IBM/ibm-common-service-operator/controllers/bootstrap"
	"github.com/IBM/ibm-common-service-operator/controllers/check"
	util "github.com/IBM/ibm-common-service-operator/controllers/common"
	"github.com/IBM/ibm-common-service-operator/controllers/constant"
	"github.com/IBM/ibm-common-service-operator/controllers/deploy"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(operatorv3.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	utilruntime.Must(olmv1alpha1.AddToScheme(scheme))
	utilruntime.Must(olmv1.AddToScheme(scheme))
}

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "be598e12.ibm.com",
	})
	if err != nil {
		klog.Errorf("Unable to start manager: %v", err)
		os.Exit(1)
	}

	// New bootstrap Object
	bs := bootstrap.NewBootstrap(mgr)
	operatorNs, err := util.GetOperatorNamespace()
	if err != nil {
		klog.Errorf("Getting operator namespace failed: %v", err)
		os.Exit(1)
	}

	// Create master namespace
	if operatorNs != constant.MasterNamespace {
		klog.Infof("Creating IBM Common Services master namespace: %s", constant.MasterNamespace)
		if err := bs.CreateNamespace(); err != nil {
			klog.Errorf("Failed to create master namespace: %v", err)
			os.Exit(1)
		}

		klog.Info("Creating OperatorGroup for IBM Common Services")
		if err := bs.CreateOperatorGroup(); err != nil {
			klog.Errorf("Failed to create OperatorGroup for IBM Common Services: %v", err)
			os.Exit(1)
		}
	}

	if operatorNs == constant.MasterNamespace || operatorNs == constant.ClusterOperatorNamespace {
		klog.Info("Creating CommonService CR in master namespace")
		if err = bs.CreateCsCR(); err != nil {
			klog.Errorf("Failed to create CommonService CR: %v", err)
			os.Exit(1)
		}

		// Check IAM pods status
		go check.IamStatus(mgr)

		if err = (&controllers.CommonServiceReconciler{
			Client:    mgr.GetClient(),
			Reader:    mgr.GetAPIReader(),
			Manager:   deploy.NewDeployManager(mgr),
			Bootstrap: bootstrap.NewBootstrap(mgr),
			Log:       ctrl.Log.WithName("controllers").WithName("CommonService"),
			Scheme:    mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			klog.Errorf("Unable to create controller CommonService: %v", err)
			os.Exit(1)
		}
		// +kubebuilder:scaffold:builder
	} else {
		klog.Info("Creating common service operator subscription in master namespace")
		if err = bs.CreateCsSubscription(); err != nil {
			klog.Errorf("Failed to create common service operator subscription: %v", err)
			os.Exit(1)
		}
	}

	klog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Errorf("Problem running manager: %v", err)
		os.Exit(1)
	}
}
