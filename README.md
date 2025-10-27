# NetBird Standalone Terraform Importer

A modular, standalone tool to import NetBird resources into Terraform configuration files. This tool isolates the NetBird provider functionality to avoid gRPC version conflicts with the main Terraformer project and follows the existing codebase patterns for maximum modularity.

## 🚀 Features

- **Modular Architecture**: Clean separation of concerns with dedicated generators for each resource type
- **Complete Resource Coverage**: Imports all NetBird resource types with full attribute support
- **Smart Reference Resolution**: Automatically converts resource IDs to Terraform references
- **Zero External Dependencies**: Uses only Go standard library (no gRPC dependencies)
- **Configurable Server URLs**: Support for custom NetBird server endpoints
- **Clean Terraform Output**: Generates properly formatted, human-readable Terraform files

## 📋 Prerequisites

- Go 1.21 or later
- NetBird API token with appropriate permissions
- Network access to your NetBird API server

## 🛠️ Installation

### Option 1: Build from Source
```bash
cd netbird-standalone
go build -o netbird-importer .
```

### Option 2: Cross-Platform Build
```bash
# Build for multiple platforms
make build-all

# Or build for current platform
make build
```

## 🔧 Configuration

The tool supports two configuration methods:

### Environment Variables (Recommended)
```bash
export NB_PAT="your-personal-access-token"
export NB_MANAGEMENT_URL="https://netbird.monitorbit.xyz:33073"  # Optional
export DEBUG="true"  # Optional, for debugging API requests
```

### Default Values
- **Management URL**: Defaults to `https://api.netbird.io` if not specified
- **Output Directory**: Defaults to `generated/` if not specified

## 🚀 Usage

### Basic Usage
```bash
# Import to default 'generated' directory
./netbird-importer

# Import to custom directory
./netbird-importer my-terraform-config

# Show detailed help
./netbird-importer --help
```

### Example with Custom Server
```bash
export NB_PAT="pat_your_token_here"
export NB_MANAGEMENT_URL="https://netbird.monitorbit.xyz:33073"
./netbird-importer terraform-config
```

## 📁 Generated Files Structure

The tool creates a complete Terraform configuration with the following files:

```
generated/
├── provider.tf       # Provider configuration with your server URL and token
├── group.tf         # NetBird group resources
├── peer.tf          # NetBird peer resources  
├── user.tf          # NetBird user resources
├── policy.tf        # NetBird policy resources with rules
├── route.tf         # NetBird route resources
└── setup_key.tf     # NetBird setup key resources
```

## 🎯 Resource Types & Features

| Resource Type | Features | Terraform References |
|---------------|----------|---------------------|
| **Groups** | Basic group configuration | Referenced by other resources |
| **Peers** | SSH settings, login expiration | Group membership via references |
| **Users** | Roles, auto-groups, status | Auto-group references |
| **Policies** | Rules, port ranges, bidirectional | Source/destination group references |
| **Routes** | Network routing, masquerading | Peer and group references |
| **Setup Keys** | Expiration, usage limits | Auto-group assignments |

## 🔄 Post-Import Workflow

1. **Navigate to generated directory**
   ```bash
   cd generated  # or your custom directory
   ```

2. **Initialize Terraform**
   ```bash
   terraform init
   ```

3. **Review the plan**
   ```bash
   terraform plan
   ```

4. **Customize if needed**
   - Edit `.tf` files to match your requirements
   - Update provider configuration for production use

5. **Apply configuration**
   ```bash
   terraform apply
   ```

## 🏗️ Architecture & Modularity

The codebase follows a clean, modular architecture:

```
netbird-standalone/
├── main.go                 # CLI interface and orchestration
├── config.go              # Configuration management
├── service.go             # NetBird API client
├── terraform_generator.go # Terraform file generation
├── generators.go          # Resource-specific generators
├── Makefile              # Build automation
└── README.md             # Documentation
```

### Key Design Principles

1. **Single Responsibility**: Each generator handles one resource type
2. **Dependency Injection**: Services are injected into generators
3. **Interface-Based**: Common `ResourceGenerator` interface
4. **Reference Resolution**: Automatic ID-to-name conversion for Terraform references
5. **Error Handling**: Graceful failure with detailed error messages

## 🔌 API Endpoints

The tool interacts with the following NetBird API endpoints:

| Endpoint | Purpose | Generator |
|----------|---------|-----------|
| `/api/groups` | Fetch groups | GroupsGenerator |
| `/api/peers` | Fetch peers | PeersGenerator |
| `/api/users` | Fetch users | UsersGenerator |
| `/api/policies` | Fetch policies | PoliciesGenerator |
| `/api/routes` | Fetch routes | RoutesGenerator |
<!-- | `/api/setup-keys` | Fetch setup keys | SetupKeysGenerator | -->

## 🐛 Troubleshooting

### Authentication Issues
```bash
# Verify token is set
echo $NB_PAT

# Test API access manually
curl -H "Authorization: Token $NB_PAT" \
     $NB_MANAGEMENT_URL/api/groups
```

### Network Configuration
```bash
# Test connectivity to custom server
curl -k https://netbird.monitorbit.xyz:33073/api/groups

# Enable debug mode for detailed request information
export DEBUG=true
./netbird-importer
```

### Common Issues

| Issue | Solution |
|-------|----------|
| `NB_PAT environment variable is required` | Set the NetBird Personal Access Token |
| `failed to fetch X: API request failed with status 401` | Check token validity and permissions |
| `failed to fetch X: API request failed with status 404` | Verify management URL is correct |
| Empty resources in output | Check API permissions for the token |
| Getting HTML instead of JSON | Verify the management URL points to API, not dashboard |

## 🎛️ Advanced Configuration

### Custom Provider Configuration
The generated `provider.tf` includes examples for variable-based configuration:

```hcl
provider "netbird" {
  management_url = "https://netbird.monitorbit.xyz:33073"
  token          = var.netbird_token
}
```

### Resource Filtering
Currently, all accessible resources are imported. For selective import, modify the `main.go` file to comment out unwanted generators.

## 🤝 Contributing

The modular architecture makes it easy to extend:

1. **Add new resource types**: Implement the `ResourceGenerator` interface
2. **Enhance existing generators**: Modify individual generator files
3. **Improve Terraform output**: Update `terraform_generator.go`
4. **Add configuration options**: Extend `config.go`

## 📄 License

This tool follows the same license as the main Terraformer project.