## 安装本体

```bash
# 二进制（官方下载，最简单）
wget https://releases.hashicorp.com/terraform/1.9.5/terraform_1.9.5_linux_amd64.zip
unzip terraform_1.9.5_linux_amd64.zip -d /usr/local/bin/
terraform -version

# 包管理器
yum install -y yum-utils
yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
yum install -y terraform
```

## 版本管理器 tfenv（多版本并行）

不同项目用的 Terraform / Provider 版本不同，**tfenv 按目录切版本**：

```bash
git clone https://github.com/tfutils/tfenv.git ~/.tfenv
export PATH="$HOME/.tfenv/bin:$PATH"
tfenv install 1.9.5 && tfenv use 1.9.5
```

## 第一个项目：初始化与最小可用

```bash
mkdir demo && cd demo

cat > main.tf <<EOF
terraform {
  required_version = ">= 1.9"
  required_providers {
    tencentcloud = { source = "tencentcloudstack/tencentcloud", version = "~> 1.81" }
  }
}

provider "tencentcloud" {
  secret_id  = var.secret_id
  secret_key = var.secret_key
  region     = "ap-guangzhou"
}
EOF

terraform init        # 拉取 provider，这一步走网络，内网配镜像
terraform validate    # 语法检查
```

内网/国内加速 provider 下载（`~/.terraformrc`）：

```hcl
provider_installation {
  network_mirror { url = "https://mirrors.tencent.com/terraform/" }
}
```

## 离线安装 Provider

外网机器 `terraform init` 成功后，把
`~/.terraform.d/providers/`（或 `.terraform/providers/`）整个目录拷进内网同路径，
再配 `filesystem_mirror` 的 `provider_installation`。

## 验证流程

```bash
terraform plan        # 只读预演，永远先 plan 后 apply
terraform apply -auto-approve=false
terraform state list  # 当前管理的资源
```