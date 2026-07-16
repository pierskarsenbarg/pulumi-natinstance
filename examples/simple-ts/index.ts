import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as awsx from "@pulumi/awsx";
import * as nat from "@pierskarsenbarg/natinstance";

const vpc = new awsx.ec2.Vpc("old-version-vpc", {
    cidrBlock: "10.0.0.0/16",
    numberOfAvailabilityZones: 2,
    subnetSpecs: [{
        type: awsx.ec2.SubnetType.Public,
        name: "public-ecs-subnet",
    }],
    tags: {
        name: "pk-ecs-connect"
    },
    natGateways: {
        strategy: "None"
    },
    subnetStrategy: awsx.ec2.SubnetAllocationStrategy.Auto
});

const instance = new nat.NatInstance("nat", {
    instanceType: aws.ec2.InstanceType.T3_Small,
    vpcId: vpc.vpcId
})