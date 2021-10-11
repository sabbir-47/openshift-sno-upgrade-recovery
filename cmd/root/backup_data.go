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
func launchInitialBackupJobs(KubeconfigPath string, Spoke string) error {
	client, err := client.New(KubeconfigPath, Spoke)
	if err != nil {
		return err
	}

	if ! client.SpokeClusterExists() {
		log.Warn(fmt.Sprintf("Cluster %s does not exist", Spoke))
	}
	return nil
}

var backupInitialDataCmd = &cobra.Command{
	Use:   "backupInitialData",
	Short: "It will trigger the backup of initial data for a given spoke. Needs to be run at hub cluster",

	RunE: func(cmd *cobra.Command, args []string) error {
		KubeconfigPath, _ := cmd.Flags().GetString("KubeconfigPath")

		// validate that file exists
		if _, err := os.Stat(KubeconfigPath); os.IsNotExist(err) {
			log.Error(err)
			return err
		}

		// launch jobs for live image and release
		Spoke, _ := cmd.Flags().GetString("Spoke")
		err := launchInitialBackupJobs(KubeconfigPath, Spoke)
		return err
	},
}

func init() {
	
	rootCmd.AddCommand(backupInitialDataCmd)

	backupInitialDataCmd.Flags().StringP("KubeconfigPath", "k", "", "Path to kubeconfig file")
	backupInitialDataCmd.MarkFlagRequired("KubeconfigPath")

	backupInitialDataCmd.Flags().StringP("Spoke", "s", "", "Path to the spoke cluster")
	backupInitialDataCmd.MarkFlagRequired("Spoke")

	// bind to viper
	viper.BindPFlag("KubeconfigPath", backupInitialDataCmd.Flags().Lookup("KubeconfigPath"))
	viper.BindPFlag("Spoke", backupInitialDataCmd.Flags().Lookup("Spoke"))
}
