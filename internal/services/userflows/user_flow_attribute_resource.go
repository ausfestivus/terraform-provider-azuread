// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package userflows

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/identity/stable/userflowattribute"
	"github.com/hashicorp/go-azure-sdk/sdk/nullable"
	"github.com/hashicorp/terraform-provider-azuread/internal/clients"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/consistency"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf/validation"
)

func userFlowAttributeResource() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		CreateContext: userFlowAttributeResourceCreate,
		ReadContext:   userFlowAttributeResourceRead,
		UpdateContext: userFlowAttributeResourceUpdate,
		DeleteContext: userFlowAttributeResourceDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(5 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(5 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"display_name": {
				Description: "The display name of the user flow attribute.",
				Type:        pluginsdk.TypeString,
				Required:    true,
				ForceNew:    true,
			},

			"data_type": {
				Description:  "The data type of the user flow attribute",
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(stable.PossibleValuesForIdentityUserFlowAttributeDataType(), false),
			},

			"description": {
				Description: "The description of the user flow attribute that is shown to the user at the time of sign-up",
				Type:        pluginsdk.TypeString,
				Required:    true,
			},

			"attribute_type": {
				Description: "The type of the user flow attribute",
				Type:        pluginsdk.TypeString,
				Computed:    true,
			},
		},
	}
}

func userFlowAttributeResourceCreate(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).UserFlows.UserFlowAttributeClient

	displayName := d.Get("display_name").(string)

	options := userflowattribute.ListUserFlowAttributesOperationOptions{
		Filter: pointer.To(fmt.Sprintf("displayName eq '%s'", displayName)),
	}
	if resp, err := client.ListUserFlowAttributes(ctx, options); err != nil {
		return tf.ErrorDiagF(err, "Checking for existing user flow attribute")
	} else if resp.Model != nil {
		for _, r := range *resp.Model {
			model := r.IdentityUserFlowAttribute()
			if model.Id != nil && strings.EqualFold(model.DisplayName.GetOrZero(), displayName) {
				return tf.ImportAsExistsDiag("azuread_user_flow_attribute", *model.Id)
			}
		}
	}

	attr := stable.BaseIdentityUserFlowAttributeImpl{
		DataType:    pointer.To(stable.IdentityUserFlowAttributeDataType(d.Get("data_type").(string))),
		Description: nullable.NoZero(d.Get("description").(string)),
		DisplayName: nullable.NoZero(displayName),
	}

	resp, err := client.CreateUserFlowAttribute(ctx, attr, userflowattribute.DefaultCreateUserFlowAttributeOperationOptions())
	if err != nil {
		return tf.ErrorDiagF(err, "Creating user flow attribute")
	}

	if resp.Model == nil {
		return tf.ErrorDiagF(errors.New("model was nil"), "Creating user flow attribute")
	}

	userFlowAttr := resp.Model.IdentityUserFlowAttribute()

	if userFlowAttr.Id == nil || *userFlowAttr.Id == "" {
		return tf.ErrorDiagF(errors.New("API returned user flow attribute with nil ID"), "Bad API Response")
	}

	id := stable.NewIdentityUserFlowAttributeID(*userFlowAttr.Id)
	d.SetId(id.IdentityUserFlowAttributeId)

	// Now ensure we can retrieve the attribute consistently
	if err = consistency.WaitForUpdate(ctx, func(ctx context.Context) (*bool, error) {
		resp, err := client.GetUserFlowAttribute(ctx, id, userflowattribute.DefaultGetUserFlowAttributeOperationOptions())
		if err != nil {
			if response.WasNotFound(resp.HttpResponse) {
				return pointer.To(false), nil
			}
			return pointer.To(false), err
		}
		return pointer.To(resp.Model != nil), nil
	}); err != nil {
		return tf.ErrorDiagF(err, "Waiting for creation of %s", id)
	}

	return userFlowAttributeResourceRead(ctx, d, meta)
}

func userFlowAttributeResourceUpdate(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).UserFlows.UserFlowAttributeClient
	id := stable.NewIdentityUserFlowAttributeID(d.Id())

	attr := stable.BaseIdentityUserFlowAttributeImpl{
		Description: nullable.NoZero(d.Get("description").(string)),
	}

	if _, err := client.UpdateUserFlowAttribute(ctx, id, attr, userflowattribute.DefaultUpdateUserFlowAttributeOperationOptions()); err != nil {
		return tf.ErrorDiagF(err, "Could not update user flow attribute with ID: %q", id)
	}

	return userFlowAttributeResourceRead(ctx, d, meta)
}

func userFlowAttributeResourceRead(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).UserFlows.UserFlowAttributeClient
	id := stable.NewIdentityUserFlowAttributeID(d.Id())

	resp, err := client.GetUserFlowAttribute(ctx, id, userflowattribute.DefaultGetUserFlowAttributeOperationOptions())
	if err != nil {
		if response.WasNotFound(resp.HttpResponse) {
			log.Printf("[DEBUG] %s was not found - removing from state!", id)
			d.SetId("")
			return nil
		}
		return tf.ErrorDiagF(err, "Retrieving %s", id)
	}

	if resp.Model == nil {
		return tf.ErrorDiagF(errors.New("model was nil"), "Creating user flow attribute")
	}

	userFlowAttr := resp.Model.IdentityUserFlowAttribute()

	tf.Set(d, "attribute_type", pointer.From(userFlowAttr.UserFlowAttributeType))
	tf.Set(d, "data_type", pointer.From(userFlowAttr.DataType))
	tf.Set(d, "description", userFlowAttr.Description.GetOrZero())
	tf.Set(d, "display_name", userFlowAttr.DisplayName.GetOrZero())

	return nil
}

func userFlowAttributeResourceDelete(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).UserFlows.UserFlowAttributeClient
	id := stable.NewIdentityUserFlowAttributeID(d.Id())

	if _, err := client.DeleteUserFlowAttribute(ctx, id, userflowattribute.DefaultDeleteUserFlowAttributeOperationOptions()); err != nil {
		return tf.ErrorDiagF(err, "Deleting %s", id)
	}

	if err := consistency.WaitForDeletion(ctx, func(ctx context.Context) (*bool, error) {
		if resp, err := client.GetUserFlowAttribute(ctx, id, userflowattribute.DefaultGetUserFlowAttributeOperationOptions()); err != nil {
			if response.WasNotFound(resp.HttpResponse) {
				return pointer.To(false), nil
			}
			return nil, err
		}
		return pointer.To(true), nil
	}); err != nil {
		return tf.ErrorDiagF(err, "Waiting for deletion of %s", id)
	}

	return nil
}
