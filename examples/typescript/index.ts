import * as exedev from "@firstatlast/exedev";

// A persistent Linux VM on exe.dev.
const vm = new exedev.Vm("dev", {
    image: "ubuntu:22.04",
    cpu: 2,
    memory: "4GB",
    disk: "20GB",
    tags: ["dev", "pulumi"],
    env: { NODE_ENV: "development" },
    comment: "managed by pulumi",
});

export const name = vm.vmName;
export const url = vm.httpsUrl;
export const ssh = vm.sshDest;
export const status = vm.status;
