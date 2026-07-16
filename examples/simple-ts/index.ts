import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as awsx from "@pulumi/awsx";
import * as nat from "@pierskarsenbarg/natinstance";

const vpc = new awsx.ec2.Vpc("test-nat-vpc", {
    cidrBlock: "10.0.0.0/16",
    numberOfAvailabilityZones: 2,
    subnetSpecs: [{
        type: awsx.ec2.SubnetType.Public,
        name: "public-nat-subnet",
    }, {
        type: awsx.ec2.SubnetType.Private,
        name: "private-nat-subnet"
    }],
    tags: {
        name: "pk-nat-test",
        owner: "team-ce"
    },
    natGateways: {
        strategy: "None"
    },
    subnetStrategy: awsx.ec2.SubnetAllocationStrategy.Auto
});

const natinstance = new nat.NatInstance("nat", {
    instanceType: aws.ec2.InstanceType.T3_Small,
    vpcId: vpc.vpcId
});

export const mynat = natinstance;

const ami = aws.ec2.getAmiOutput({
    mostRecent: true,
    owners: ["amazon"],
    filters: [{
        name: "name",
        values: ["al2023-ami-*-x86_64"],
    }],
});

const eicSecurityGroup = new aws.ec2.SecurityGroup("eic-sg", {
    vpcId: vpc.vpcId,
    egress: [{
        protocol: "-1",
        fromPort: 0,
        toPort: 0,
        cidrBlocks: ["0.0.0.0/0"],
    }],
});

const privateInstanceSecurityGroup = new aws.ec2.SecurityGroup("private-instance-sg", {
    vpcId: vpc.vpcId,
    ingress: [{
        protocol: "tcp",
        fromPort: 22,
        toPort: 22,
        securityGroups: [eicSecurityGroup.id],
    }],
    egress: [{
        protocol: "-1",
        fromPort: 0,
        toPort: 0,
        cidrBlocks: ["0.0.0.0/0"],
    }],
});

const privateInstance = new aws.ec2.Instance("private-nat-test-instance", {
    ami: ami.id,
    instanceType: aws.ec2.InstanceType.T3_Micro,
    subnetId: vpc.privateSubnetIds.apply(ids => ids[0]),
    vpcSecurityGroupIds: [privateInstanceSecurityGroup.id],
    tags: {
        Name: "private-nat-test-instance",
    },
});

const eicEndpoint = new aws.ec2transitgateway.InstanceConnectEndpoint("nat-test-eic", {
    subnetId: vpc.privateSubnetIds.apply(ids => ids[0]),
    securityGroupIds: [eicSecurityGroup.id],
});

export const privateInstanceId = privateInstance.id;
export const eicEndpointId = eicEndpoint.id;
