# Installation and upgrades

## Platform

```bash
helm install kubesentry-gateway ./charts/kubesentry-gateway -n kubesentry-system --create-namespace
kubectl get pods -n kubesentry-system
```

Open `http://<NodeIP>:30080` and set the default `admin` password. Production deployments should use HTTPS, set `console.session.cookieSecure=true`, and replace the default PostgreSQL password or use an external database.

## Member cluster

Create a cluster in Console, select installation options, and run the complete generated Helm command in the target cluster. Never retain or reuse the one-time registration token. Wait for Online status before assigning the cluster to a tenant.

## Upgrade and uninstall

Use `helm upgrade` for either chart and back up PostgreSQL first. Start member uninstall from Console so Operator removes managed resources, then delete the cluster record. Roll back with the prior chart/images and database backup; migration reversibility must be assessed per release.
