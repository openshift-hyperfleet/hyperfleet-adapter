# Adapter Configuration Files

This directory contains adapter-specific configuration files that will be mounted as ConfigMaps in the adapter pods.

## Structure

```
configs/
  ├── validation-adapter-config.yaml    # Validation adapter configuration
  ├── dns-adapter-config.yaml           # DNS adapter configuration
  └── placement-adapter-config.yaml     # Placement adapter configuration
```

## Usage

These files are automatically loaded by the Helm chart and mounted into adapter containers at:
```
/etc/adapter/config/adapter-config.yaml
```

## Configuration Format

Each configuration file follows the adapter config schema defined in:
`/data/adapter-config-template.yaml`

See the [adapter configuration documentation](../../data/adapter-config-template.yaml) for complete structure and examples.

## Creating New Adapter Configurations

1. Copy the template:
   ```bash
   cp ../../data/adapter-config-template.yaml ./my-adapter-config.yaml
   ```

2. Edit the configuration for your use case

3. Reference in `values.yaml`:
   ```yaml
   adapters:
     myAdapter:
       enabled: true
       configFile: "my-adapter-config.yaml"
   ```

## Validation

Before deploying, validate YAML syntax:

```bash
yq eval validation-adapter-config.yaml > /dev/null && echo "Valid YAML" || echo "Invalid YAML"
```

## Examples

Minimal adapter configuration:

```yaml
adapterName: example-adapter

filters:
  eventTypes:
    - "cluster.created"
    - "cluster.updated"

parameters:
  - name: clusterId
    from:
      type: event
      path: data.resourceId

preconditions: []

resources:
  - template: |
      apiVersion: v1
      kind: Namespace
      metadata:
        name: cluster-{{ .clusterId }}

post:
  parameters: []
  postActions: []
```

See individual config files in this directory for complete examples.

