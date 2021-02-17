package sfs

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/sfs/v2/shares"

	"github.com/opentelekomcloud/terraform-provider-opentelekomcloud/opentelekomcloud/common/cfg"
)

func ResourceSFSFileSystemV2() *schema.Resource {
	return &schema.Resource{
		Create: resourceSFSFileSystemV2Create,
		Read:   resourceSFSFileSystemV2Read,
		Update: resourceSFSFileSystemV2Update,
		Delete: resourceSFSFileSystemV2Delete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"region": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},
			"share_proto": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "NFS",
			},
			"size": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"is_public": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
			"availability_zone": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},
			"access_level": {
				Type:     schema.TypeString,
				Required: true,
			},
			"access_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "cert",
			},
			"access_to": {
				Type:     schema.TypeString,
				Required: true,
			},
			"share_access_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"access_rule_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"host": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"export_location": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"volume_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"share_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceSFSMetadataV2(d *schema.ResourceData) map[string]string {
	meta := make(map[string]string)
	for key, val := range d.Get("metadata").(map[string]interface{}) {
		meta[key] = val.(string)
	}
	return meta
}

func resourceSFSFileSystemV2Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*cfg.Config)
	client, err := config.SfsV2Client(config.GetRegion(d))
	if err != nil {
		return fmt.Errorf("error creating OpenTelekomCloud File Share Client: %s", err)
	}

	createOpts := shares.CreateOpts{
		ShareProto:       d.Get("share_proto").(string),
		Size:             d.Get("size").(int),
		Name:             d.Get("name").(string),
		Description:      d.Get("description").(string),
		IsPublic:         d.Get("is_public").(bool),
		Metadata:         resourceSFSMetadataV2(d),
		AvailabilityZone: d.Get("availability_zone").(string),
	}

	share, err := shares.Create(client, createOpts).Extract()
	if err != nil {
		return fmt.Errorf("error creating OpenTelekomCloud File Share: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"creating"},
		Target:     []string{"available"},
		Refresh:    waitForSFSFileStatus(client, share.ID),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error applying access rules to share file: %s", err)
	}

	grantAccessOpts := shares.GrantAccessOpts{
		AccessLevel: d.Get("access_level").(string),
		AccessType:  d.Get("access_type").(string),
		AccessTo:    d.Get("access_to").(string),
	}

	_, err = shares.GrantAccess(client, share.ID, grantAccessOpts).ExtractAccess()
	if err != nil {
		return fmt.Errorf("error applying access rules to share file: %s", err)
	}

	d.SetId(share.ID)

	return resourceSFSFileSystemV2Read(d, meta)
}

func resourceSFSFileSystemV2Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*cfg.Config)
	client, err := config.SfsV2Client(config.GetRegion(d))
	if err != nil {
		return fmt.Errorf("error creating OpenTelekomCloud File Share: %s", err)
	}

	share, err := shares.Get(client, d.Id()).Extract()
	if err != nil {
		if _, ok := err.(golangsdk.ErrDefault404); ok {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("error retrieving OpenTelekomCloud Shares: %s", err)
	}
	mErr := multierror.Append(nil,
		d.Set("name", share.Name),
		d.Set("share_proto", share.ShareProto),
		d.Set("status", share.Status),
		d.Set("size", share.Size),
		d.Set("description", share.Description),
		d.Set("share_type", share.ShareType),
		d.Set("volume_type", share.VolumeType),
		d.Set("is_public", share.IsPublic),
		d.Set("availability_zone", share.AvailabilityZone),
		d.Set("region", config.GetRegion(d)),
		d.Set("export_location", share.ExportLocation),
		d.Set("host", share.Host),
	)

	// NOTE: This tries to remove system metadata.
	metadata := make(map[string]string)
	for key, val := range share.Metadata {
		if strings.HasPrefix(key, "#sfs") {
			continue
		}
		if strings.Contains(key, "enterprise_project_id") || strings.Contains(key, "share_used") {
			continue
		}
		metadata[key] = val
	}
	if err := d.Set("metadata", metadata); err != nil {
		return err
	}

	rules, err := shares.ListAccessRights(client, d.Id()).ExtractAccessRights()
	if err != nil {
		if _, ok := err.(golangsdk.ErrDefault404); ok {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("error retrieving OpenTelekomCloud Shares: %s", err)
	}

	if len(rules) == 0 {
		return nil
	}
	rule := rules[0]
	mErr = multierror.Append(mErr,
		d.Set("share_access_id", rule.ID),
		d.Set("access_rule_status", rule.State),
		d.Set("access_to", rule.AccessTo),
		d.Set("access_type", rule.AccessType),
		d.Set("access_level", rule.AccessLevel),
	)

	if mErr.ErrorOrNil() != nil {
		return mErr
	}

	return nil
}

func resourceSFSFileSystemV2Update(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*cfg.Config)
	client, err := config.SfsV2Client(config.GetRegion(d))
	if err != nil {
		return fmt.Errorf("error updating OpenTelekomCloud Share File: %s", err)
	}
	var updateOpts shares.UpdateOpts

	if d.HasChange("description") || d.HasChange("name") {
		updateOpts.DisplayName = d.Get("name").(string)
		updateOpts.DisplayDescription = d.Get("description").(string)

		_, err = shares.Update(client, d.Id(), updateOpts).Extract()
		if err != nil {
			return fmt.Errorf("error updating OpenTelekomCloud Share File: %s", err)
		}
	}
	if d.HasChange("access_to") || d.HasChange("access_level") || d.HasChange("access_type") {
		deleteAccessOpts := shares.DeleteAccessOpts{AccessID: d.Get("share_access_id").(string)}
		if err := shares.DeleteAccess(client, d.Id(), deleteAccessOpts).Err; err != nil {
			return fmt.Errorf("error changing access rules for share file: %s", err)
		}

		grantAccessOpts := shares.GrantAccessOpts{
			AccessLevel: d.Get("access_level").(string),
			AccessType:  d.Get("access_type").(string),
			AccessTo:    d.Get("access_to").(string),
		}

		log.Printf("[DEBUG] Grant Access Rules: %#v", grantAccessOpts)
		_, err := shares.GrantAccess(client, d.Id(), grantAccessOpts).ExtractAccess()
		if err != nil {
			return fmt.Errorf("error changing access rules for share file: %s", err)
		}
	}

	if d.HasChange("size") {
		oldSizeRaw, newSizeRaw := d.GetChange("size")
		newSize := newSizeRaw.(int)
		if oldSizeRaw.(int) < newSize {
			expandOpts := shares.ExpandOpts{OSExtend: shares.OSExtendOpts{NewSize: newSize}}
			if err := shares.Expand(client, d.Id(), expandOpts).ExtractErr(); err != nil {
				return fmt.Errorf("error expanding OpenTelekomCloud Share File size: %s", err)
			}
		} else {
			shrinkOpts := shares.ShrinkOpts{OSShrink: shares.OSShrinkOpts{NewSize: newSize}}
			if err := shares.Shrink(client, d.Id(), shrinkOpts).ExtractErr(); err != nil {
				return fmt.Errorf("error shrinking OpenTelekomCloud Share File size: %s", err)
			}
		}
	}

	return resourceSFSFileSystemV2Read(d, meta)
}

func resourceSFSFileSystemV2Delete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*cfg.Config)
	client, err := config.SfsV2Client(config.GetRegion(d))
	if err != nil {
		return fmt.Errorf("error creating OpenTelekomCloud Shared File: %s", err)
	}
	err = shares.Delete(client, d.Id()).ExtractErr()
	if err != nil {
		return fmt.Errorf("error deleting OpenTelekomCloud Shared File: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"available", "deleting"},
		Target:     []string{"deleted"},
		Refresh:    waitForSFSFileStatus(client, d.Id()),
		Timeout:    d.Timeout(schema.TimeoutDelete),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error deleting OpenTelekomCloud Share File: %s", err)
	}

	d.SetId("")
	return nil
}

func waitForSFSFileStatus(client *golangsdk.ServiceClient, shareID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		share, err := shares.Get(client, shareID).Extract()
		if err != nil {
			if _, ok := err.(golangsdk.ErrDefault404); ok {
				log.Printf("[INFO] Successfully deleted OpenTelekomCloud shared File %s", shareID)
				return share, "deleted", nil
			}
			return nil, "", err
		}
		return share, share.Status, nil
	}
}