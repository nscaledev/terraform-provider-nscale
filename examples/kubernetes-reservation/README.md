# NKS cluster on reserved capacity (two stacks)

Two root modules with separate state, because the reservation and the cluster
have different lifecycles:

| Stack | Contains | Apply time | Changes |
| --- | --- | --- | --- |
| [`capacity/`](capacity/main.tf) | `nscale_reservation`, with `prevent_destroy` | up to ~60m | rarely |
| [`cluster/`](cluster/main.tf) | network, NKS cluster, reservation-backed node pool | ~10-35m | often |

```
capacity/  nscale_reservation ──── reservation_id ───┐
                                                     ▼
cluster/   network ─► cluster ─► node pool (provisioning_mode = "reservation")
                                     └─ NKS claims hosts as a placement → placement_id
```

## Usage

```sh
# 1. Capacity: once. Waits until the reservation is provisioned (up to 90m).
cd capacity
terraform init && terraform apply
RESERVATION_ID=$(terraform output -raw reservation_id)

# 2. Cluster: as often as you like, against the same reservation.
cd ../cluster
terraform init && terraform apply -var reservation_id="$RESERVATION_ID"
```

The cluster stack refuses to plan until the reservation is `provisioned`, and
when NKS has no platform release in the reservation's region. Its network, and
so the cluster and pool, are created in the reservation's region and project.

## Tearing down

- `terraform destroy` in `cluster/` removes the pool, cluster and network. The
  reservation, and the capacity it holds, stay.
- To give the capacity back, remove `prevent_destroy` from `capacity/main.tf`,
  then `terraform destroy` there. Destroy `cluster/` first: deleting a
  reservation also deletes every placement taken from it.
