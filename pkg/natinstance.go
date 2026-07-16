// Package pkg implements the NatInstance component resource.
package pkg

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	vpcModule "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/vpc"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type NatInstance struct{}

type NatInstanceArgs struct {
	InstanceType pulumi.StringInput `pulumi:"instanceType"`
	VpcId        pulumi.StringInput `pulumi:"vpcId,optional"`
	Cidr         pulumi.StringInput `pulumi:"cidr,optional"`
	SubnetId     pulumi.StringInput `pulumi:"subnetId,optional"`
}

func (fna *NatInstanceArgs) Annotate(a infer.Annotator) {
	a.Describe(&fna.InstanceType, "Instance type of the NAT instance")
	a.Describe(&fna.VpcId, "Id of the VPC that the NAT instance will be inside. Will select the default VPC for the region if not set.")
	a.Describe(&fna.Cidr, "CIDR blocks that you want the NAT instance to be available to. Will use the CIDR block for the VPC otherwise")
	a.Describe(&fna.SubnetId, "Public subnet ID where the NAT instance will be created. If not specified then one will be selected from the VPC.")
}

type NatInstanceState struct {
	pulumi.ResourceState
	SecurityGroupId pulumi.IDOutput `pulumi:"securityGroupId"`
}

func (kca *NatInstanceState) Annotate(a infer.Annotator) {
	a.Describe(&kca.SecurityGroupId, "Security group ID to attach to any resource that needs to send traffic through the NAT instance")
}

func hasPublicRoute(routes []ec2.GetRouteTableRoute) bool {
	idx := slices.IndexFunc(routes, func(r ec2.GetRouteTableRoute) bool {
		return (r.CidrBlock == "0.0.0.0/0" && strings.HasPrefix(r.GatewayId, "igw-"))
	})
	return (idx != -1)
}

func getSubnetIdsFromVpcId(vpcId string, ctx *pulumi.Context) ([]string, error) {
	subnetIds, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
		Filters: []ec2.GetSubnetsFilter{
			{
				Name:   "vpc-id",
				Values: []string{vpcId},
			},
		},
	})
	if err != nil {
		return []string{""}, err
	}
	return subnetIds.Ids, nil
}

func getSubnetsFromRouteTableIds(routeTableIds []string, ctx *pulumi.Context) ([]string, error) {
	routeTablesMap := make(map[string]any)
	var subnetIds []string
	for _, id := range routeTableIds {
		routeTable, err := ec2.LookupRouteTable(ctx, &ec2.LookupRouteTableArgs{
			Filters: []ec2.GetRouteTableFilter{
				{
					Name:   "route-table-id",
					Values: []string{id},
				},
			},
		})
		if err != nil {
			return subnetIds, err
		}
		routeTablesMap[id] = routeTable
	}

	for _, route := range routeTablesMap {
		routeValue := route.(*ec2.LookupRouteTableResult)
		if hasPublicRoute(routeValue.Routes) {
			subnetIds = append(subnetIds, routeValue.Associations[0].SubnetId)
		}
	}

	return subnetIds, nil
}

func (f *NatInstance) Construct(ctx *pulumi.Context, name, typ string, args NatInstanceArgs, opts pulumi.ResourceOption) (
	*NatInstanceState, error,
) {
	comp := &NatInstanceState{}
	err := ctx.RegisterComponentResource(typ, name, comp, opts)
	if err != nil {
		return nil, err
	}

	var vpc ec2.LookupVpcResultOutput

	if args.VpcId == nil {
		vpc = ec2.LookupVpcOutput(ctx, ec2.LookupVpcOutputArgs{
			Default: pulumi.BoolPtr(true),
		})
	} else {
		vpc = ec2.LookupVpcOutput(ctx, ec2.LookupVpcOutputArgs{
			Id: args.VpcId,
		})
	}

	var subnetId pulumi.StringOutput
	if args.SubnetId != nil {
		subnetId = args.SubnetId.ToStringOutput()
	} else {
		routeTableIds := ec2.GetRouteTablesOutput(ctx, ec2.GetRouteTablesOutputArgs{
			VpcId: vpc.Id(),
		})

		vpcHasMultipleRoutes := pulumi.All(routeTableIds.Ids(), vpc.MainRouteTableId()).ApplyT(
			func(args []any) bool {
				routeTblIds := args[0].([]string)
				vpcMainRouteTableId := args[1].(string)
				if len(routeTblIds) == 1 && vpcMainRouteTableId == routeTblIds[0] {
					return false
				}
				return true
			})

		subnetIds := pulumi.All(vpcHasMultipleRoutes, routeTableIds.Ids(), vpc.Id()).ApplyT(
			func(args []any) ([]string, error) {
				var subnetIds []string
				var err error
				multipleRoutes := args[0].(bool)
				routeTbleIds := args[1].([]string)
				if multipleRoutes {
					subnetIds, err = getSubnetsFromRouteTableIds(routeTbleIds, ctx)
				} else {
					vpcId := args[2].(string)
					subnetIds, err = getSubnetIdsFromVpcId(vpcId, ctx)
				}
				if err != nil {
					return nil, err
				}
				slices.Sort(subnetIds)
				return subnetIds, nil
			}).(pulumi.StringArrayOutput)

		subnetId = subnetIds.Index(pulumi.Int(0))
	}

	ingressCidr := vpc.CidrBlock()
	if args.Cidr != nil {
		ingressCidr = args.Cidr.ToStringOutput()
	}

	securitygroup, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-natsecuritygroup", name), &ec2.SecurityGroupArgs{
		VpcId:       vpc.Id(),
		Description: pulumi.String("Security group for FCK NAT instance"),
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	_, err = vpcModule.NewSecurityGroupIngressRule(ctx, "ingress", &vpcModule.SecurityGroupIngressRuleArgs{
		SecurityGroupId: securitygroup.ID(),
		CidrIpv4:        ingressCidr,
		FromPort:        pulumi.Int(0),
		ToPort:          pulumi.Int(0),
		IpProtocol:      pulumi.String("-1"),
	}, pulumi.Parent(securitygroup))
	if err != nil {
		return nil, err
	}

	_, err = vpcModule.NewSecurityGroupEgressRule(ctx, "egress", &vpcModule.SecurityGroupEgressRuleArgs{
		SecurityGroupId: securitygroup.ID(),
		CidrIpv4:        pulumi.String("0.0.0.0/0"),
		FromPort:        pulumi.Int(0),
		ToPort:          pulumi.Int(0),
		IpProtocol:      pulumi.String("-1"),
	}, pulumi.Parent(securitygroup))
	if err != nil {
		return nil, err
	}

	natInterface, err := ec2.NewNetworkInterface(ctx, fmt.Sprintf("%s-natnetworkinterface", name), &ec2.NetworkInterfaceArgs{
		SubnetId:        subnetId,
		SecurityGroups:  pulumi.StringArray{securitygroup.ID()},
		SourceDestCheck: pulumi.BoolPtr(false),
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	assumeRolePolicy, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
		Statements: []iam.GetPolicyDocumentStatement{
			{
				Actions: []string{
					"sts:AssumeRole",
				},
				Principals: []iam.GetPolicyDocumentStatementPrincipal{
					{
						Type: "Service",
						Identifiers: []string{
							"ec2.amazonaws.com",
						},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		return nil, err
	}

	// Create IAM Role
	natRole, err := iam.NewRole(ctx, fmt.Sprintf("%s-fckRole", name), &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(assumeRolePolicy.Json),
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	natRolePolicy := iam.GetPolicyDocumentOutput(ctx, iam.GetPolicyDocumentOutputArgs{
		Statements: iam.GetPolicyDocumentStatementArray{
			iam.GetPolicyDocumentStatementArgs{
				Actions: pulumi.StringArray{
					pulumi.String("ec2:AttachNetworkInterface"),
					pulumi.String("ec2:ModifyNetworkInterfaceAttribute"),
				},
				Effect:    pulumi.String("Allow"),
				Resources: pulumi.StringArray{pulumi.String("*")},
			},
			iam.GetPolicyDocumentStatementArgs{
				Actions: pulumi.StringArray{
					pulumi.String("ec2:AssociateAddress"),
					pulumi.String("ec2:DisassociateAddress"),
				},
				Effect: pulumi.String("Allow"),
				Resources: pulumi.StringArray{
					pulumi.String("*"),
				},
			},
		},
	})

	_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-rpa", name), &iam.RolePolicyArgs{
		Role:   natRole.Name,
		Policy: natRolePolicy.Json(),
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	instanceProfile, err := iam.NewInstanceProfile(ctx, fmt.Sprintf("%s-instanceprofile", name), &iam.InstanceProfileArgs{
		Role: natRole.Name,
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	userData := pulumi.Sprintf(`#!/bin/bash
	              echo "eni_id=%s" >> /etc/fck-nat.conf
				  service fck-nat restart
	`, natInterface.ID())
	useDataB64 := userData.ApplyT(func(data string) string {
		return base64.StdEncoding.EncodeToString([]byte(data))
	}).(pulumi.StringOutput)

	ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
		Owners: []string{
			"568608671756",
		},
		MostRecent: new(true),
		Filters: []ec2.GetAmiFilter{
			{
				Name: "name",
				Values: []string{
					"fck-nat-al2023-*",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	launchTemplate, err := ec2.NewLaunchTemplate(ctx, fmt.Sprintf("%s-launchtemplate", name), &ec2.LaunchTemplateArgs{
		ImageId:      pulumi.String(ami.Id),
		InstanceType: args.InstanceType,
		IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileArgs{
			Arn: instanceProfile.Arn,
		},
		VpcSecurityGroupIds: pulumi.StringArray{
			securitygroup.ID(),
		},
		UserData: useDataB64,
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	_, err = autoscaling.NewGroup(ctx, fmt.Sprintf("%s-asg", name), &autoscaling.GroupArgs{
		MaxSize:         pulumi.Int(1),
		MinSize:         pulumi.Int(1),
		DesiredCapacity: pulumi.Int(1),
		LaunchTemplate: &autoscaling.GroupLaunchTemplateArgs{
			Id:      launchTemplate.ID(),
			Version: pulumi.String("$Latest"),
		},
		VpcZoneIdentifiers: pulumi.StringArray{
			subnetId,
		},
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, err
	}

	comp.SecurityGroupId = securitygroup.ID()

	return comp, nil
}
