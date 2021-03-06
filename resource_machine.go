package main

import (
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/joyent/gosdc/cloudapi"
	"reflect"
	"regexp"
	"time"
)

var (
	machineStateRunning = "running"
	machineStateStopped = "stopped"

	machineStateChangeTimeout       = 10 * time.Minute
	machineStateChangeCheckInterval = 10 * time.Second

	resourceMachineMetadataKeys = map[string]string{
		// semantics: "schema_name": "metadata_name"
		"root_authorized_keys": "root_authorized_keys",
		"user_script":          "user-script",
		"user_data":            "user-data",
		"administrator_pw":     "administrator-pw",
	}
)

func resourceMachine() *schema.Resource {
	return &schema.Resource{
		Create: wrapCallback(resourceMachineCreate),
		Exists: wrapExistsCallback(resourceMachineExists),
		Read:   wrapCallback(resourceMachineRead),
		Update: wrapCallback(resourceMachineUpdate),
		Delete: wrapCallback(resourceMachineDelete),

		Schema: map[string]*schema.Schema{
			"name": {
				Description:  "friendly name",
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: resourceMachineValidateName,
			},
			"type": {
				Description: "machine type (smartmachine or virtualmachine)",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"state": {
				Description: "current state of the machine",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"dataset": {
				Description: "dataset URN the machine was provisioned with",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"memory": {
				Description: "amount of memory the machine has (in Mb)",
				Type:        schema.TypeInt,
				Computed:    true,
			},
			"disk": {
				Description: "amount of disk the machine has (in Gb)",
				Type:        schema.TypeInt,
				Computed:    true,
			},
			"ips": {
				Description: "IP addresses the machine has",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"tags": {
				Description: "machine tags",
				Type:        schema.TypeMap,
				Optional:    true,
			},
			"created": {
				Description: "when the machine was created",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"updated": {
				Description: "when the machine was update",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"package": {
				Description: "name of the package to use on provisioning",
				Type:        schema.TypeString,
				Required:    true,
			},
			"image": {
				Description: "image UUID",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				// TODO: validate that the UUID is valid
			},
			"primaryip": {
				Description: "the primary (public) IP address for the machine",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"networks": {
				Description: "desired network IDs",
				Type:        schema.TypeList,
				Optional:    true,
				// TODO: this really should ForceNew but the Network IDs don't seem to
				// be returned by the API, meaning if we track them here TF will replace
				// the resource on every run.
				// ForceNew:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"firewall_enabled": {
				Description: "enable firewall for this machine",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},

			// computed resources from metadata
			"root_authorized_keys": {
				Description: "authorized keys for the root user on this machine",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"user_script": {
				Description: "user script to run on boot (every boot on SmartMachines)",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"user_data": {
				Description: "copied to machine on boot",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"administrator_pw": {
				Description: "administrator's initial password (Windows only)",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"metadata": {
				Type:     schema.TypeList,
				Optional: true,
				Removed:  "Please use root attributes instead of metadata",
				Elem: &schema.Resource{
					Schema: func() map[string]*schema.Schema {
						m := map[string]*schema.Schema{}

						for schemaName, apiName := range resourceMachineMetadataKeys {
							m[apiName] = &schema.Schema{
								Description: "deprecated",
								Optional:    true,
								Type:        schema.TypeString,
								Removed:     fmt.Sprintf("Please use %s instead of metadata.%s", schemaName, apiName),
							}
						}

						return m
					}(),
				},
			},
		},
	}
}

func resourceMachineCreate(d *schema.ResourceData, config *Config) error {
	api, err := config.Cloud()
	if err != nil {
		return err
	}

	var networks []string
	for _, network := range d.Get("networks").([]interface{}) {
		networks = append(networks, network.(string))
	}

	metadata := map[string]string{}
	for schemaName, metadataKey := range resourceMachineMetadataKeys {
		if v, ok := d.GetOk(schemaName); ok {
			metadata[metadataKey] = v.(string)
		}
	}

	tags := map[string]string{}
	for k, v := range d.Get("tags").(map[string]interface{}) {
		tags[k] = v.(string)
	}

	machine, err := api.CreateMachine(cloudapi.CreateMachineOpts{
		Name:            d.Get("name").(string),
		Package:         d.Get("package").(string),
		Image:           d.Get("image").(string),
		Networks:        networks,
		Metadata:        metadata,
		Tags:            tags,
		FirewallEnabled: d.Get("firewall_enabled").(bool),
	})
	if err != nil {
		return err
	}

	err = waitForMachineState(api, machine.Id, machineStateRunning, machineStateChangeTimeout)
	if err != nil {
		return err
	}

	// refresh state after it provisions
	d.SetId(machine.Id)
	err = resourceMachineRead(d, config)
	if err != nil {
		return err
	}

	return nil
}

func resourceMachineExists(d *schema.ResourceData, config *Config) (bool, error) {
	api, err := config.Cloud()
	if err != nil {
		return false, err
	}

	machine, err := api.GetMachine(d.Id())

	return machine != nil && err == nil, err
}

func resourceMachineRead(d *schema.ResourceData, config *Config) error {
	api, err := config.Cloud()
	if err != nil {
		return err
	}

	machine, err := api.GetMachine(d.Id())
	if err != nil {
		return err
	}

	d.SetId(machine.Id)
	d.Set("name", machine.Name)
	d.Set("type", machine.Type)
	d.Set("state", machine.State)
	d.Set("dataset", machine.Dataset)
	d.Set("memory", machine.Memory)
	d.Set("disk", machine.Disk)
	d.Set("ips", machine.IPs)
	d.Set("tags", machine.Tags)
	d.Set("created", machine.Created)
	d.Set("updated", machine.Updated)
	d.Set("package", machine.Package)
	d.Set("image", machine.Image)
	d.Set("primaryip", machine.PrimaryIP)
	d.Set("networks", machine.Networks)
	// d.Set("firewall_enabled", machine.FirewallEnabled) // but that field doesn't exist...

	// computed attributes from metadata
	for schemaName, metadataKey := range resourceMachineMetadataKeys {
		d.Set(schemaName, machine.Metadata[metadataKey])
	}

	// Initialize connection info to enable remote-exec
	d.SetConnInfo(map[string]string{
		"type": "ssh",
		"host": machine.PrimaryIP,
	})

	return nil
}

func resourceMachineUpdate(d *schema.ResourceData, config *Config) error {
	api, err := config.Cloud()
	if err != nil {
		return err
	}

	d.Partial(true)

	if d.HasChange("name") {
		if err := api.RenameMachine(d.Id(), d.Get("name").(string)); err != nil {
			return err
		}

		err := waitFor(
			func() (bool, error) {
				machine, err := api.GetMachine(d.Id())
				return machine.Name == d.Get("name").(string), err
			},
			machineStateChangeCheckInterval,
			1*time.Minute,
		)
		if err != nil {
			return err
		}

		d.SetPartial("name")
	}

	if d.HasChange("tags") {
		tags := map[string]string{}
		for k, v := range d.Get("tags").(map[string]interface{}) {
			tags[k] = v.(string)
		}

		if len(tags) == 0 {
			err = api.DeleteMachineTags(d.Id())
		} else {
			_, err = api.ReplaceMachineTags(d.Id(), tags)
		}
		if err != nil {
			return err
		}

		err = waitFor(
			func() (bool, error) {
				machine, err := api.GetMachine(d.Id())
				return reflect.DeepEqual(machine.Tags, tags), err
			},
			machineStateChangeCheckInterval,
			1*time.Minute,
		)
		if err != nil {
			return err
		}

		d.SetPartial("tags")
	}

	if d.HasChange("package") {
		if err := api.ResizeMachine(d.Id(), d.Get("package").(string)); err != nil {
			return err
		}

		err := waitFor(
			func() (bool, error) {
				machine, err := api.GetMachine(d.Id())
				return machine.Package == d.Get("package").(string) && machine.State == machineStateRunning, err
			},
			machineStateChangeCheckInterval,
			machineStateChangeTimeout,
		)
		if err != nil {
			return err
		}

		d.SetPartial("package")
	}

	if d.HasChange("firewall_enabled") {
		if d.Get("firewall_enabled").(bool) {
			err = api.EnableFirewallMachine(d.Id())
		} else {
			err = api.DisableFirewallMachine(d.Id())
		}
		if err != nil {
			return err
		}

		d.SetPartial("firewall_enabled")
	}

	// metadata stuff
	metadata := map[string]string{}
	for schemaName, metadataKey := range resourceMachineMetadataKeys {
		if d.HasChange(schemaName) {
			metadata[metadataKey] = d.Get(schemaName).(string)
		}
	}
	if len(metadata) > 0 {
		_, err := api.UpdateMachineMetadata(d.Id(), metadata)
		if err != nil {
			return err
		}

		err = waitFor(
			func() (bool, error) {
				machine, err := api.GetMachine(d.Id())
				return reflect.DeepEqual(machine.Metadata, metadata), err
			},
			machineStateChangeCheckInterval,
			1*time.Minute,
		)
		if err != nil {
			return err
		}

		for schemaName := range resourceMachineMetadataKeys {
			if d.HasChange(schemaName) {
				d.SetPartial(schemaName)
			}
		}
	}

	d.Partial(false)

	err = resourceMachineRead(d, config)
	if err != nil {
		return err
	}

	return nil
}

func resourceMachineDelete(d *schema.ResourceData, config *Config) error {
	api, err := config.Cloud()
	if err != nil {
		return err
	}

	state, err := readMachineState(api, d.Id())
	if state != machineStateStopped {
		err = api.StopMachine(d.Id())
		if err != nil {
			return err
		}

		waitForMachineState(api, d.Id(), machineStateStopped, machineStateChangeTimeout)
	}

	err = api.DeleteMachine(d.Id())
	if err != nil {
		return err
	}

	d.SetId("")

	return nil
}

func readMachineState(api *cloudapi.Client, id string) (string, error) {
	machine, err := api.GetMachine(id)
	if err != nil {
		return "", err
	}

	return machine.State, nil
}

// waitForMachineState waits for a machine to be in the desired state (waiting
// some seconds between each poll). If it doesn't reach the state within the
// duration specified in `timeout`, it returns ErrMachineStateTimeout.
func waitForMachineState(api *cloudapi.Client, id, state string, timeout time.Duration) error {
	return waitFor(
		func() (bool, error) {
			currentState, err := readMachineState(api, id)
			return currentState == state, err
		},
		machineStateChangeCheckInterval,
		machineStateChangeTimeout,
	)
}

func resourceMachineValidateName(value interface{}, name string) (warnings []string, errors []error) {
	warnings = []string{}
	errors = []error{}

	r := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\_\.\-]*$`)
	if !r.Match([]byte(value.(string))) {
		errors = append(errors, fmt.Errorf(`"%s" is not a valid %s`, value.(string), name))
	}

	return warnings, errors
}
