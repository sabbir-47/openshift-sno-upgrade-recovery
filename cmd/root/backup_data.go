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
	"fmt"

	"github.com/redhat-ztp/openshift-ai-trigger-backup/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
	//"net/url"
	"os"
	//"strings"
)

// it will connect to kubernetes and retrieve the live image
// and release image needed to back up in the spoke cluster
// it will create policies triggering jobs on the spoke
func launchInitialBackupJobs(client client.Client, Spoke string, BinaryImage string, BackupPath string) error {
	// retrieve version of the cluster, and checks for live img on the config
	liveImg, err := client.GetRootFSUrl()
	if err != nil {
		log.Error(fmt.Sprintf("Cannot retrieve root fs from %s", Spoke))
		return nil
	}

	// create placement rule for spoke if it doesn't exist
	_ = client.CreatePlacementRule()

	if liveImg != "" {
		// create policy for backing up live image
		_ = client.LaunchLiveImageBackup(liveImg)

	}

	// now extract the release image
	releaseImg, err := client.GetReleaseImage()

	if err != nil {
		log.Error(fmt.Sprintf("Cannot retrieve root fs from %s", Spoke))
		return nil
	}

	if releaseImg != "" {
		// create policy for backup up release image
		client.LaunchReleaseImageBackup(releaseImg)
	}

	client.WaitForCompletion()

	return nil
}

// check if a policy with the expected name is existing and remove it, and dependencies
func removePreviousPolicies(client client.Client, Spoke string, BinaryImage string, BackupPath string) error {
	// check if cluster exists and has been imported
	if !client.SpokeClusterExists() {
		log.Warn(fmt.Sprintf("Cluster %s does not exist", Spoke))
		return nil
	}

	err := client.RemovePreviousResources()
	if err != nil {
		return err
	}

	return nil

}

var backupInitialDataCmd = &cobra.Command{
	Use:   "backupInitialData",
	Short: "It will trigger the backup of initial data for a given spoke. Needs to be run at hub cluster",

	RunE: func(cmd *cobra.Command, args []string) error {
		KubeconfigPath, _ := cmd.Flags().GetString("KubeconfigPath")

		if KubeconfigPath != "" {
			// validate that file exists
			if _, err := os.Stat(KubeconfigPath); os.IsNotExist(err) {
				log.Error(err)
				return err
			}
		}

		// get spoke cluster
		Spoke, _ := cmd.Flags().GetString("Spoke")

		// get config options
		BinaryImage, _ := cmd.Flags().GetString("BinaryImage")
		BackupPath, _ := cmd.Flags().GetString("BackupPath")

		client, err := client.New(KubeconfigPath, Spoke, BinaryImage, BackupPath)
		if err != nil {
			return err
		}

		// remove previous policies if they are already there
		err = removePreviousPolicies(client, Spoke, BinaryImage, BackupPath)
		if err != nil {
			return err
		}

		err = launchInitialBackupJobs(client, Spoke, BinaryImage, BackupPath)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {

	rootCmd.AddCommand(backupInitialDataCmd)

	backupInitialDataCmd.Flags().StringP("KubeconfigPath", "k", "", "Path to kubeconfig file")

	backupInitialDataCmd.Flags().StringP("Spoke", "s", "", "Path to the spoke cluster")
	backupInitialDataCmd.MarkFlagRequired("Spoke")

	backupInitialDataCmd.Flags().StringP("BinaryImage", "l", "quay.io/yrobla/openshift-ai-image-backup", "Path to the binary for image backups")
	backupInitialDataCmd.Flags().StringP("BackupPath", "p", "/var/recovery", "Path where to store the backups")

	// bind to viper
	viper.BindPFlag("KubeconfigPath", backupInitialDataCmd.Flags().Lookup("KubeconfigPath"))
	viper.BindPFlag("Spoke", backupInitialDataCmd.Flags().Lookup("Spoke"))
	viper.BindPFlag("BinaryImage", backupInitialDataCmd.Flags().Lookup("BinaryImage"))
	viper.BindPFlag("BackupPath", backupInitialDataCmd.Flags().Lookup("BackupPath"))
}
