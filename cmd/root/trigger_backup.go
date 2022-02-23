/*
Copyright © 2021 Yolanda Robla <yroblamo@redhat.com>

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
package root

import (
	"time"

	metaclient1 "github.com/redhat-ztp/openshift-ai-trigger-backup/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/errors"

	log "github.com/sirupsen/logrus"
	//"net/url"
	//"strings"
)

func launchBackupJobs(client metaclient1.Client) error {

	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)
	// check whether the spoke exists
	if !client.SpokeClusterExists() {
		log.Errorf("Cluster %s does not exist", client.Spoke)
		return nil
	}
	log.Info("Cluster exists!")

	log.Info("Creating Kubernetes objects")

	// TO DO:
	// 1. If client can't create any object it should delete all the created object - done
	// 2. Query with a function if the k8s job is succesfully finished.
	// 3. Once done, we must cleanup artifacts at spoke.

	// Launch k8s job, if it fails to launch it must delete the objects it created.

	err := client.LaunchKubernetesObjects(metaclient1.ActionCreateTemplates, "create")
	if err != nil {
		log.Errorf("Couldn't launch k8s ManagedClusterAction objects in the %s cluster err: %s", client.Spoke, err)
		log.Info("Deleting all mca objects")
		if _, err = client.ManageObjects(metaclient1.ActionCreateTemplates, metaclient1.MCA, "delete"); err != nil {
			log.Errorf("Couldn't delete k8s ManagedClusterAction objects in the %s cluster err: %s", client.Spoke, err)
			return err
		}
		return err
	}
	log.Info("Successfully created all K8s mca objects")

	// create managedclusterview object
	_, err = client.ManageObjects(metaclient1.ViewCreateTemplates, metaclient1.MCV, "get")
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = client.ManageObjects(metaclient1.ViewCreateTemplates, metaclient1.MCV, "delete")
			if err != nil {
				log.Errorf("Couldn't delete existing ManagedclusterView object in the %s cluster err: %s", client.Spoke, err)
				return err
			}
		}
		if errors.IsNotFound(err) {
			err = client.LaunchKubernetesObjects(metaclient1.ViewCreateTemplates, "create")
			if err != nil {
				log.Errorf("Couldn't launch k8s ManagedclusterView object the %s cluster err: %s", client.Spoke, err)
				return err
			}
		}
	}
	log.Info("Successfully created ManagedclusterView object")

	time.Sleep(1 * time.Second)
	// check job status via managedclusterview
	err = client.CheckStatus(metaclient1.MCV)
	if err != nil {
		log.Errorf("Couldn't verify the job status, err: %s", err)
		return nil
	}
	time.Sleep(1 * time.Second)

	// delete managedclusterview
	_, err = client.ManageObjects(metaclient1.ViewCreateTemplates, metaclient1.MCV, "delete")
	if err != nil {
		log.Errorf("Couldn't delete existing ManagedclusterView object in the %s cluster err: %s", client.Spoke, err)
		return err
	}

	time.Sleep(1 * time.Second)
	//delete the namespace in the spoke, which will delete the completed job and associated pod.
	err = client.LaunchKubernetesObjects(metaclient1.JobDeleteTemplates, "create")
	if err != nil {
		log.Errorf("Couldn't launch k8 objects in the %s cluster err: %s", client.Spoke, err)
		return err
	}
	log.Info("Successfully deleted all Kubernetes objects")

	return nil
}

var triggerBackupCmd = &cobra.Command{
	Use:   "triggerBackup",
	Short: "It will trigger the backup of the resources in the spoke cluster",

	RunE: func(cmd *cobra.Command, args []string) error {
		// get spoke cluster
		Spoke, _ := cmd.Flags().GetString("Spoke")
		BackupPath, _ := cmd.Flags().GetString("BackupPath")
		KubeconfigPath, _ := cmd.Flags().GetString("KubeconfigPath")

		client, err := metaclient1.New(Spoke, BackupPath, KubeconfigPath)
		if err != nil {
			return err
		}

		err = launchBackupJobs(client)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {

	rootCmd.AddCommand(triggerBackupCmd)

	triggerBackupCmd.Flags().StringP("Spoke", "s", "", "Name of the Spoke cluster")
	triggerBackupCmd.MarkFlagRequired("Spoke")

	triggerBackupCmd.Flags().StringP("KubeconfigPath", "k", "", "Path to kubeconfig file")
	triggerBackupCmd.MarkFlagRequired("KubeconfigPath")

	triggerBackupCmd.Flags().StringP("BackupPath", "p", "/var/recovery", "Path of recovery partition where backups will be stored")

	// bind to viper
	viper.BindPFlag("Spoke", triggerBackupCmd.Flags().Lookup("Spoke"))
	viper.BindPFlag("BackupPath", triggerBackupCmd.Flags().Lookup("BackupPath"))
	viper.BindPFlag("KubeconfigPath", triggerBackupCmd.Flags().Lookup("KubeconfigPath"))
}
