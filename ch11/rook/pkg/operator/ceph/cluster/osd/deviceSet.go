/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package osd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// deviceSet is the processed version of the StorageClassDeviceSet
type deviceSet struct {
	// Name is the name of the volume source
	Name string
	// PVCSources
	PVCSources map[string]v1.PersistentVolumeClaimVolumeSource
	// CrushDeviceClass represents the crush device class for an OSD
	CrushDeviceClass string
	// CrushInitialWeight represents initial OSD weight in TiB units
	CrushInitialWeight string
	// CrushPrimaryAffinity represents initial OSD primary-affinity within range [0, 1]
	CrushPrimaryAffinity string
	// Size represents the size requested for the PVC
	Size string
	// Resources requests/limits for the devices
	Resources v1.ResourceRequirements
	// Placement constraints for the device daemons
	Placement cephv1.Placement
	// Placement constraints for the device preparation
	PreparePlacement *cephv1.Placement
	// Provider-specific device configuration
	Config map[string]string
	// Portable represents OSD portability across the hosts
	Portable bool
	// TuneSlowDeviceClass Tune the OSD when running on a slow Device Class
	TuneSlowDeviceClass bool
	// TuneFastDeviceClass Tune the OSD when running on a fast Device Class
	TuneFastDeviceClass bool
	// Scheduler name for OSD pod placement
	SchedulerName string
	// Whether to encrypt the deviceSet
	Encrypted bool
}

// PrepareStorageClassDeviceSets is only exposed for testing purposes
func (c *Cluster) PrepareStorageClassDeviceSets() error {
	errors := newProvisionErrors()
	c.prepareStorageClassDeviceSets(errors)
	if len(errors.errors) > 0 {
		// return the first error
		return errors.errors[0]
	}
	return nil
}

func (c *Cluster) prepareStorageClassDeviceSets(errs *provisionErrors) {
	c.deviceSets = []deviceSet{}

	existingPVCs, uniqueOSDsPerDeviceSet, err := GetExistingPVCs(c.clusterInfo.Context, c.context, c.clusterInfo.Namespace)
	if err != nil {
		errs.addError("failed to detect existing OSD PVCs. %v", err)
		return
	}

	// Iterate over deviceSet
	for _, deviceSet := range c.spec.Storage.StorageClassDeviceSets {
		if err := controller.CheckPodMemory(cephv1.ResourcesKeyPrepareOSD, deviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
			errs.addError("failed to provision OSDs on PVC for storageClassDeviceSet %q. %v", deviceSet.Name, err)
			continue
		}
		// Check if the volume claim template is specified
		if len(deviceSet.VolumeClaimTemplates) == 0 {
			errs.addError("failed to provision OSDs on PVC for storageClassDeviceSet %q. no volumeClaimTemplate is specified. user must specify a volumeClaimTemplate", deviceSet.Name)
			continue
		}

		// Iterate through existing PVCs to ensure they are up-to-date, no metadata pvcs are missing, etc
		highestExistingID := -1
		countInDeviceSet := 0
		if existingIDs, ok := uniqueOSDsPerDeviceSet[deviceSet.Name]; ok {
			logger.Infof("verifying PVCs exist for %d OSDs in device set %q", existingIDs.Len(), deviceSet.Name)
			for existingID := range existingIDs {
				pvcID, err := strconv.Atoi(existingID)
				if err != nil {
					errs.addError("invalid PVC index %q found for device set %q", existingID, deviceSet.Name)
					continue
				}
				// keep track of the max PVC index found so we know what index to start with for new OSDs
				if pvcID > highestExistingID {
					highestExistingID = pvcID
				}
				deviceSet := c.createDeviceSetPVCsForIndex(deviceSet, existingPVCs, pvcID, errs)
				c.deviceSets = append(c.deviceSets, deviceSet)
			}
			countInDeviceSet = existingIDs.Len()
		}
		// Create new PVCs if we are not yet at the expected count
		// No new PVCs will be created if we have too many
		pvcsToCreate := deviceSet.Count - countInDeviceSet
		if pvcsToCreate > 0 {
			logger.Infof("creating %d new PVCs for device set %q", pvcsToCreate, deviceSet.Name)
		}
		for i := 0; i < pvcsToCreate; i++ {
			pvcID := highestExistingID + i + 1
			deviceSet := c.createDeviceSetPVCsForIndex(deviceSet, existingPVCs, pvcID, errs)
			c.deviceSets = append(c.deviceSets, deviceSet)
			countInDeviceSet++
		}
	}
}

func (c *Cluster) createDeviceSetPVCsForIndex(newDeviceSet cephv1.StorageClassDeviceSet, existingPVCs map[string]*v1.PersistentVolumeClaim, setIndex int, errs *provisionErrors) deviceSet {
	// Create the PVC source for each of the data, metadata, and other types of templates if defined.
	pvcSources := map[string]v1.PersistentVolumeClaimVolumeSource{}

	var dataSize string
	var crushDeviceClass string
	var crushInitialWeight string
	var crushPrimaryAffinity string
	typesFound := sets.NewString()
	for _, pvcTemplate := range newDeviceSet.VolumeClaimTemplates {
		if pvcTemplate.Name == "" {
			// For backward compatibility a blank name must be treated as a data volume
			pvcTemplate.Name = bluestorePVCData
		}
		if typesFound.Has(pvcTemplate.Name) {
			errs.addError("found duplicate volume claim template %q for device set %q", pvcTemplate.Name, newDeviceSet.Name)
			continue
		}
		typesFound.Insert(pvcTemplate.Name)

		pvc, err := c.createDeviceSetPVC(existingPVCs, newDeviceSet.Name, pvcTemplate, setIndex)
		if err != nil {
			errs.addError("failed to provision PVC for device set %q index %d. %v", newDeviceSet.Name, setIndex, err)
			continue
		}

		// The PVC type must be from a predefined set such as "data", "metadata", and "wal". These names must be enforced if the wal/db are specified
		// with a separate device, but if there is a single volume template we can assume it is always the data template.
		pvcType := pvcTemplate.Name
		if len(newDeviceSet.VolumeClaimTemplates) == 1 {
			pvcType = bluestorePVCData
		}

		if pvcType == bluestorePVCData {
			pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
			dataSize = pvcSize.String()
			crushDeviceClass = pvcTemplate.Annotations["crushDeviceClass"]
		}
		crushInitialWeight = pvcTemplate.Annotations["crushInitialWeight"]
		crushPrimaryAffinity = pvcTemplate.Annotations["crushPrimaryAffinity"]

		pvcSources[pvcType] = v1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvc.GetName(),
			ReadOnly:  false,
		}
	}

	return deviceSet{
		Name:                 newDeviceSet.Name,
		Resources:            newDeviceSet.Resources,
		Placement:            newDeviceSet.Placement,
		PreparePlacement:     newDeviceSet.PreparePlacement,
		Config:               newDeviceSet.Config,
		Size:                 dataSize,
		PVCSources:           pvcSources,
		Portable:             newDeviceSet.Portable,
		TuneSlowDeviceClass:  newDeviceSet.TuneSlowDeviceClass,
		TuneFastDeviceClass:  newDeviceSet.TuneFastDeviceClass,
		SchedulerName:        newDeviceSet.SchedulerName,
		CrushDeviceClass:     crushDeviceClass,
		CrushInitialWeight:   crushInitialWeight,
		CrushPrimaryAffinity: crushPrimaryAffinity,
		Encrypted:            newDeviceSet.Encrypted,
	}
}

func (c *Cluster) createDeviceSetPVC(existingPVCs map[string]*v1.PersistentVolumeClaim, deviceSetName string, pvcTemplate v1.PersistentVolumeClaim, setIndex int) (*v1.PersistentVolumeClaim, error) {
	// old labels and PVC ID for backward compatibility
	pvcID := legacyDeviceSetPVCID(deviceSetName, setIndex)

	// check for the existence of the pvc
	existingPVC, ok := existingPVCs[pvcID]
	if !ok {
		// The old name of the PVC didn't exist, now try the new PVC name and label
		pvcID = deviceSetPVCID(deviceSetName, pvcTemplate.GetName(), setIndex)
		existingPVC = existingPVCs[pvcID]
	}
	pvc := makeDeviceSetPVC(deviceSetName, pvcID, setIndex, pvcTemplate, c.clusterInfo.Namespace)
	err := c.clusterInfo.OwnerInfo.SetControllerReference(pvc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to osd pvc %q", pvc.Name)
	}

	if existingPVC != nil {
		logger.Infof("OSD PVC %q already exists", existingPVC.Name)

		// Update the PVC in case the size changed
		k8sutil.ExpandPVCIfRequired(c.clusterInfo.Context, c.context.Client, pvc, existingPVC)
		return existingPVC, nil
	}

	// No PVC found, creating a new one
	deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Create(c.clusterInfo.Context, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create PVC %q for device set %q", pvc.Name, deviceSetName)
	}
	logger.Infof("successfully provisioned PVC %q", deployedPVC.Name)

	return deployedPVC, nil
}

func makeDeviceSetPVC(deviceSetName, pvcID string, setIndex int, pvcTemplate v1.PersistentVolumeClaim, namespace string) *v1.PersistentVolumeClaim {
	pvcLabels := makeStorageClassDeviceSetPVCLabel(deviceSetName, pvcID, setIndex)

	// Add user provided labels to pvcTemplates
	for k, v := range pvcTemplate.GetLabels() {
		pvcLabels[k] = v
	}

	// pvc naming format rook-ceph-osd-<deviceSetName>-<SetNumber>-<PVCIndex>-<generatedSuffix>
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			// Use a generated name to avoid the possibility of two OSDs being created with the same ID.
			// If one is removed and a new one is created later with the same ID, the OSD would fail to start.
			GenerateName: pvcID,
			Namespace:    namespace,
			Labels:       pvcLabels,
			Annotations:  pvcTemplate.Annotations,
		},
		Spec: pvcTemplate.Spec,
	}
}

// GetExistingPVCs fetches the list of OSD PVCs
func GetExistingPVCs(ctx context.Context, clusterdContext *clusterd.Context, namespace string) (map[string]*v1.PersistentVolumeClaim, map[string]sets.String, error) {
	selector := metav1.ListOptions{LabelSelector: CephDeviceSetPVCIDLabelKey}
	pvcs, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, selector)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to detect PVCs")
	}
	result := map[string]*v1.PersistentVolumeClaim{}
	uniqueOSDsPerDeviceSet := map[string]sets.String{}
	for i, pvc := range pvcs.Items {
		// Populate the PVCs based on their unique name across all the device sets
		pvcID := pvc.Labels[CephDeviceSetPVCIDLabelKey]
		result[pvcID] = &pvcs.Items[i]

		// Create a map of the PVC IDs available in each device set based on PVC index
		deviceSet := pvc.Labels[CephDeviceSetLabelKey]
		pvcIndex := pvc.Labels[CephSetIndexLabelKey]
		if _, ok := uniqueOSDsPerDeviceSet[deviceSet]; !ok {
			uniqueOSDsPerDeviceSet[deviceSet] = sets.NewString()
		}
		uniqueOSDsPerDeviceSet[deviceSet].Insert(pvcIndex)
	}

	return result, uniqueOSDsPerDeviceSet, nil
}

func legacyDeviceSetPVCID(deviceSetName string, setIndex int) string {
	return fmt.Sprintf("%s-%d", deviceSetName, setIndex)
}

// This is the new function that generates the labels
// It includes the pvcTemplateName in it
func deviceSetPVCID(deviceSetName, pvcTemplateName string, setIndex int) string {
	cleanName := strings.Replace(pvcTemplateName, " ", "-", -1)
	return fmt.Sprintf("%s-%s-%d", deviceSetName, cleanName, setIndex)
}
