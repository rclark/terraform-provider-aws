package connect

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/connect"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceInstanceStorageConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceInstanceStorageConfigCreate,
		ReadContext:   resourceInstanceStorageConfigRead,
		UpdateContext: resourceInstanceStorageConfigUpdate,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"association_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"instance_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 100),
			},
			"resource_type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(connect.InstanceStorageResourceType_Values(), false),
			},
			"storage_config": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"s3_config": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"bucket_name": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringLenBetween(1, 128),
									},
									"bucket_prefix": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringLenBetween(1, 128),
									},
									"encryption_config": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"encryption_type": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validation.StringInSlice(connect.EncryptionType_Values(), false),
												},
												"key_id": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: verify.ValidARN,
												},
											},
										},
									},
								},
							},
						},
						"storage_type": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice(connect.StorageType_Values(), false),
						},
					},
				},
			},
		},
	}
}

func resourceInstanceStorageConfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).ConnectConn

	instanceId := d.Get("instance_id").(string)
	resourceType := d.Get("resource_type").(string)

	input := &connect.AssociateInstanceStorageConfigInput{
		InstanceId:    aws.String(instanceId),
		ResourceType:  aws.String(resourceType),
		StorageConfig: expandStorageConfig(d.Get("storage_config").([]interface{})),
	}

	log.Printf("[DEBUG] Creating Connect Instance Storage Config %s", input)
	output, err := conn.AssociateInstanceStorageConfigWithContext(ctx, input)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error creating Connect Instance Storage Config for Connect Instance (%s,%s): %w", instanceId, resourceType, err))
	}

	if output == nil || output.AssociationId == nil {
		return diag.FromErr(fmt.Errorf("error creating Connect Instance Storage Config for Connect Instance (%s,%s): empty output", instanceId, resourceType))
	}

	d.SetId(fmt.Sprintf("%s:%s:%s", instanceId, aws.StringValue(output.AssociationId), resourceType))

	return resourceInstanceStorageConfigRead(ctx, d, meta)
}

func resourceInstanceStorageConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).ConnectConn

	instanceId, associationId, resourceType, err := InstanceStorageConfigParseId(d.Id())

	if err != nil {
		return diag.FromErr(err)
	}

	resp, err := conn.DescribeInstanceStorageConfigWithContext(ctx, &connect.DescribeInstanceStorageConfigInput{
		AssociationId: aws.String(associationId),
		InstanceId:    aws.String(instanceId),
		ResourceType:  aws.String(resourceType),
	})

	if !d.IsNewResource() && tfawserr.ErrCodeEquals(err, connect.ErrCodeResourceNotFoundException) {
		log.Printf("[WARN] Connect Instance Storage Config (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return diag.FromErr(fmt.Errorf("error getting Connect Instance Storage Config (%s): %w", d.Id(), err))
	}

	if resp == nil || resp.StorageConfig == nil {
		return diag.FromErr(fmt.Errorf("error getting Connect Instance Storage Config (%s): empty response", d.Id()))
	}

	storageConfig := resp.StorageConfig

	d.Set("association_id", storageConfig.AssociationId)
	d.Set("instance_id", instanceId)
	d.Set("resource_type", resourceType)

	if err := d.Set("storage_config", flattenStorageConfig(storageConfig)); err != nil {
		return diag.Errorf("setting storage_config: %s", err)
	}

	return nil
}

func resourceInstanceStorageConfigUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).ConnectConn

	instanceId, associationId, resourceType, err := InstanceStorageConfigParseId(d.Id())

	if err != nil {
		return diag.FromErr(err)
	}

	input := &connect.UpdateInstanceStorageConfigInput{
		AssociationId: aws.String(associationId),
		InstanceId:    aws.String(instanceId),
		ResourceType:  aws.String(resourceType),
	}

	if d.HasChange("storage_config") {
		input.StorageConfig = expandStorageConfig(d.Get("storage_config").([]interface{}))
	}

	_, err = conn.UpdateInstanceStorageConfigWithContext(ctx, input)

	if err != nil {
		return diag.FromErr(fmt.Errorf("[ERROR] Error updating Instance Storage Config (%s): %w", d.Id(), err))
	}

	return resourceInstanceStorageConfigRead(ctx, d, meta)
}

func InstanceStorageConfigParseId(id string) (string, string, string, error) {
	parts := strings.SplitN(id, ":", 3)

	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("unexpected format of ID (%s), expected instanceId:associationId:resourceType", id)
	}

	return parts[0], parts[1], parts[2], nil
}

func expandStorageConfig(tfList []interface{}) *connect.InstanceStorageConfig {
	if len(tfList) == 0 || tfList[0] == nil {
		return nil
	}

	tfMap, ok := tfList[0].(map[string]interface{})
	if !ok {
		return nil
	}

	result := &connect.InstanceStorageConfig{
		StorageType: aws.String(tfMap["storage_type"].(string)),
	}

	if v, ok := tfMap["s3_config"].([]interface{}); ok && len(v) > 0 {
		result.S3Config = exapandS3Config(v)
	}

	return result
}

func exapandS3Config(tfList []interface{}) *connect.S3Config {
	if len(tfList) == 0 || tfList[0] == nil {
		return nil
	}

	tfMap, ok := tfList[0].(map[string]interface{})
	if !ok {
		return nil
	}

	result := &connect.S3Config{
		BucketName:   aws.String(tfMap["bucket_name"].(string)),
		BucketPrefix: aws.String(tfMap["bucket_prefix"].(string)),
	}

	if v, ok := tfMap["encryption_config"].([]interface{}); ok && len(v) > 0 {
		result.EncryptionConfig = expandEncryptionConfig(v)
	}

	return result
}

func expandEncryptionConfig(tfList []interface{}) *connect.EncryptionConfig {
	if len(tfList) == 0 || tfList[0] == nil {
		return nil
	}

	tfMap, ok := tfList[0].(map[string]interface{})
	if !ok {
		return nil
	}

	result := &connect.EncryptionConfig{
		EncryptionType: aws.String(tfMap["encryption_type"].(string)),
		KeyId:          aws.String(tfMap["key_id"].(string)),
	}

	return result
}

func flattenStorageConfig(apiObject *connect.InstanceStorageConfig) []interface{} {
	if apiObject == nil {
		return []interface{}{}
	}

	values := map[string]interface{}{
		"storage_type": apiObject.StorageType,
	}

	if v := apiObject.S3Config; v != nil {
		values["s3_config"] = flattenS3Config(v)
	}

	return []interface{}{values}
}

func flattenS3Config(apiObject *connect.S3Config) []interface{} {
	if apiObject == nil {
		return []interface{}{}
	}

	values := map[string]interface{}{
		"bucket_name":   apiObject.BucketName,
		"bucket_prefix": apiObject.BucketPrefix,
	}

	if v := apiObject.EncryptionConfig; v != nil {
		values["encryption_config"] = flattenEncryptionConfig(v)
	}

	return []interface{}{values}
}

func flattenEncryptionConfig(apiObject *connect.EncryptionConfig) []interface{} {
	if apiObject == nil {
		return []interface{}{}
	}

	values := map[string]interface{}{
		"encryption_type": apiObject.EncryptionType,
		"key_id":          apiObject.KeyId,
	}

	return []interface{}{values}
}
