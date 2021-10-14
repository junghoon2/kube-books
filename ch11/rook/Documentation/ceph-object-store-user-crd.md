---
title: Object Store User CRD
weight: 2900
indent: true
---

# Ceph Object Store User CRD

Rook allows creation and customization of object store users through the custom resource definitions (CRDs). The following settings are available
for Ceph object store users.

## Sample

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: my-display-name
  quotas:
    maxBuckets: 100
    maxSize: 10G
    maxObjects: 10000
  capabilities:
    user: "*"
    bucket: "*"
```

## Object Store User Settings

### Metadata

* `name`: The name of the object store user to create, which will be reflected in the secret and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store user is created.

### Spec

* `store`: The object store in which the user will be created. This matches the name of the objectstore CRD.
* `displayName`: The display name which will be passed to the `radosgw-admin user create` command.
* `quotas`: This represents quota limitation can be set on the user (support added in Rook v1.7.3 and up).
   Please refer [here](https://docs.ceph.com/en/latest/radosgw/admin/#quota-management) for details.
    * `maxBuckets`: The maximum bucket limit for the user.
    * `maxSize`: Maximum size limit of all objects across all the user's buckets.
    * `maxObjects`: Maximum number of objects across all the user's buckets.
* `capabilities`: Ceph allows users to be given additional permissions (support added in Rook v1.7.3 and up). Due to missing APIs in go-ceph for updating the user capabilities, this setting can currently only be used during the creation of the object store user. If a user's capabilities need modified, the user must be deleted and re-created.
  See the [Ceph docs](https://docs.ceph.com/en/latest/radosgw/admin/#add-remove-admin-capabilities) for more info.
  Rook supports adding `read`, `write`, `read, write`, or `*` permissions for the following resources:
    * `users`
    * `buckets`
    * `usage`
    * `metadata`
    * `zone`
