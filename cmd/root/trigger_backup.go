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
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	metaclient1 "github.com/redhat-ztp/openshift-sno-upgrade-recovery/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/errors"

	log "github.com/sirupsen/logrus"
	//"net/url"
	//"strings"
)

type Status struct {
	ClusterName   string
	ClusterStatus string
	ClusterError  interface{}
}

// multiSpokeLaunch initiates backup to all the provided spoke clusters concurrently
// returns:			error
func multiSpokeLaunch(client metaclient1.Client) error {
	status := []Status{}
	var mu sync.Mutex
	ch := make(chan string, len(client.Spoke))
	var wg sync.WaitGroup
	log.Info("Backup will be launched concurrently on clusters: %s", client.Spoke)
	for _, v := range client.Spoke {
		wg.Add(1)
		go func(client metaclient1.Client, v string, ch chan string, wg *sync.WaitGroup) {
			retStatus, err := launchBackupJobs(client, v, ch, wg)
			mu.Lock()
			if err != nil {
				status = append(status, Status{v, retStatus, err})
			} else {
				status = append(status, Status{v, retStatus, metaclient1.NErr})
			}
			fmt.Printf("The value received from chan: %s is %s and %v\n", v, retStatus, err)
			mu.Unlock()
		}(client, v, ch, &wg)
	}
	wg.Wait()
	close(ch)

	fmt.Println(strings.Repeat("-", 85))
	w := tabwriter.NewWriter(os.Stdout, 10, 0, 0, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Cluster Name\tCluster Status\t Error\t")
	for _, v := range status {
		fmt.Fprintln(w, v.ClusterName, "\t", v.ClusterStatus, "\t", v.ClusterError, "\t")
	}
	w.Flush()
	return nil
}

// launchBackupJobs calls various Client functions to launch k8s jobs to trigger backup
// returns:			Job status, error
func launchBackupJobs(client metaclient1.Client, name string, ch chan string, wg *sync.WaitGroup) (string, error) {

	defer wg.Done()

	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)
	// check whether the spoke exists
	if !client.SpokeClusterExists(name) {
		return metaclient1.NExist, fmt.Errorf("cluster %s does not exist", name)

	}
	log.Info("Cluster exists!")

	log.Info("Creating Kubernetes objects")

	err := client.LaunchKubernetesObjects(name, metaclient1.ActionCreateTemplates)
	if err != nil {
		log.Errorf("Couldn't launch k8s ManagedClusterAction objects in the %s cluster err: %s", name, err)
		log.Info("Deleting all mca objects")
		if _, err = client.ManageObjects(name, metaclient1.ActionCreateTemplates, metaclient1.MCA, "delete"); err != nil {
			return metaclient1.Failed, fmt.Errorf("couldn't delete k8s ManagedClusterAction objects in the %s cluster err: %s", name, err)
			//	return err
		}
		return name, err
	}
	log.Info("Successfully created all K8s mca objects")

	// create managedclusterview object
	_, err = client.ManageObjects(name, metaclient1.ViewCreateTemplates, metaclient1.MCV, "get")
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = client.ManageObjects(name, metaclient1.ViewCreateTemplates, metaclient1.MCV, "delete")
			if err != nil {
				return metaclient1.Failed, fmt.Errorf("couldn't delete existing ManagedclusterView object in the %s cluster err: %s", name, err)
				//	return err
			}
		}
		if errors.IsNotFound(err) {
			err = client.LaunchKubernetesObjects(name, metaclient1.ViewCreateTemplates)
			if err != nil {
				return metaclient1.Failed, fmt.Errorf("couldn't launch k8s ManagedclusterView object the %s cluster err: %s", name, err)
				//	return err
			}
		}
	}
	log.Info("Successfully created ManagedclusterView object")

	time.Sleep(1 * time.Second)
	// check job status via managedclusterview
	err = client.JobStatus(name, metaclient1.Launch)
	if err != nil {
		return metaclient1.Failed, fmt.Errorf("couldn't verify the initiation of the job, err: %s", err)
	}

	err = client.JobStatus(name, metaclient1.Complete)
	if err != nil {
		return metaclient1.Failed, fmt.Errorf("couldn't verify if the job has finished, err: %s", err)
	}

	// delete managedclusterview
	_, err = client.ManageObjects(name, metaclient1.ViewCreateTemplates, metaclient1.MCV, "delete")
	if err != nil {
		return metaclient1.Failed, fmt.Errorf("couldn't delete existing ManagedclusterView object in the %s cluster err: %s", name, err)
	}

	//delete the namespace in the spoke, which will delete the completed job and associated pod.
	err = client.LaunchKubernetesObjects(name, metaclient1.JobDeleteTemplates)
	if err != nil {
		return metaclient1.Failed, fmt.Errorf("couldn't launch k8 objects in the %s cluster err: %s", name, err)
		//	return err
	}
	log.Info("Successfully deleted all Kubernetes objects")

	return metaclient1.Done, nil
}

var triggerBackupCmd = &cobra.Command{
	Use:   "triggerBackup",
	Short: "It will trigger the backup of the resources in the spoke cluster",

	RunE: func(cmd *cobra.Command, args []string) error {
		// get spoke cluster
		Spoke, _ := cmd.Flags().GetString("Spoke")
		splittedParam := strings.Split(Spoke, ",")
		Clustername := []string{}
		for _, v := range splittedParam {
			Clustername = append(Clustername, strings.TrimSpace(v))
		}

		BackupPath, _ := cmd.Flags().GetString("BackupPath")
		KubeconfigPath, _ := cmd.Flags().GetString("KubeconfigPath")

		client, err := metaclient1.New(Clustername, BackupPath, KubeconfigPath)
		if err != nil {
			return err
		}

		//	err = launchBackupJobs(client)
		err = multiSpokeLaunch(client)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {

	rootCmd.AddCommand(triggerBackupCmd)

	triggerBackupCmd.Flags().StringP("Spoke", "s", "", "Name of the Spoke cluster")
	if err := triggerBackupCmd.MarkFlagRequired("Spoke"); err != nil {
		return
	}

	triggerBackupCmd.Flags().StringP("KubeconfigPath", "k", "", "Path to kubeconfig file")
	if err := triggerBackupCmd.MarkFlagRequired("KubeconfigPath"); err != nil {
		return
	}

	triggerBackupCmd.Flags().StringP("BackupPath", "p", "/var/recovery", "Path of recovery partition where backups will be stored")

	// bind to viper
	_ = viper.BindPFlag("Spoke", triggerBackupCmd.Flags().Lookup("Spoke"))
	_ = viper.BindPFlag("BackupPath", triggerBackupCmd.Flags().Lookup("BackupPath"))
	_ = viper.BindPFlag("KubeconfigPath", triggerBackupCmd.Flags().Lookup("KubeconfigPath"))
}
