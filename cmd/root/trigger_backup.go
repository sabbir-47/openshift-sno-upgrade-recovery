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

	"github.com/redhat-ztp/openshift-ai-trigger-backup/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
	//"net/url"
	//"strings"
)

func launchBackupJobs(client client.Client) error {

	// check whether the spoke exists
	if !client.SpokeClusterExists() {
		log.Errorf("Cluster %s does not exist", client.Spoke)
		return nil
	}
	log.Info("Cluster exists!")

	log.Info("Creating Kubernetes objects")

	// TO DO:
	// 1. If client can't create any object it should delete all the created object
	// 2. Query with a function if the k8s job is succesfully finished.
	// 3. Once done, we must cleanup artifacts at spoke.

	// Launch k8s job, if it fails to launch it must delete the objects it created.

	err := client.LaunchKubernetesObjects(client.KubeconfigPath, "create", "hub")
	if err != nil {
		log.Errorf("Couldn't launch k8 objects in the %s cluster err: %s", client.Spoke, err)
		return err
	}
	log.Info("Successfully created all Kubernetes objects")

	time.Sleep(1 * time.Second)

	// List MCA object
	err = client.ListMCAobjects()
	if err != nil {
		log.Errorf("Couldn't list MCA objects from %s cluster; err: %s", client.Spoke, err)
		return nil
	}
	time.Sleep(1 * time.Second)
	// check job status via managedclusterview
	err = client.CheckStatus()
	if err != nil {
		log.Errorf("Couldn't verify the job status, err: %s", err)
		return nil
	}
	time.Sleep(1 * time.Second)
	// delete managedclusterview
	_, err = client.ManageView("delete")
	if err != nil {
		log.Errorf("Couldn't delete the managedclusterview job, err: %s", err)
		return nil
	}
	time.Sleep(1 * time.Second)
	//delete the namespace in the spoke, which will delete the completed job and associated pod.
	err = client.LaunchKubernetesObjects(client.KubeconfigPath, "create", "spoke")
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

		client, err := client.New(Spoke, BackupPath, KubeconfigPath)
		if err != nil {
			return err
		}

		/*	// remove previous policies if they are already there
			err = removePreviousPolicies(client, Spoke, BinaryImage, BackupPath)
			if err != nil {
				return err
			}
		*/

		err = launchBackupJobs(client)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {

	rootCmd.AddCommand(triggerBackupCmd)

	triggerBackupCmd.Flags().StringP("Spoke", "s", "", "Path to the spoke cluster")
	triggerBackupCmd.MarkFlagRequired("Spoke")

	triggerBackupCmd.Flags().StringP("KubeconfigPath", "k", "", "Path to kubeconfig file")
	triggerBackupCmd.MarkFlagRequired("KubeconfigPath")

	triggerBackupCmd.Flags().StringP("BackupPath", "p", "/var/recovery", "Path where to store the backups")

	// bind to viper
	viper.BindPFlag("Spoke", triggerBackupCmd.Flags().Lookup("Spoke"))
	viper.BindPFlag("BackupPath", triggerBackupCmd.Flags().Lookup("BackupPath"))
	viper.BindPFlag("KubeconfigPath", triggerBackupCmd.Flags().Lookup("KubeconfigPath"))
}
