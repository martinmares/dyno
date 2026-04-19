---
title: Kubernetes
weight: 30
---

# Kubernetes

## Minimal production Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myservice
  labels:
    app: myservice
spec:
  replicas: 2
  selector:
    matchLabels:
      app: myservice
  template:
    metadata:
      labels:
        app: myservice
    spec:
      containers:
        - name: myservice
          image: ghcr.io/you/myservice:abc123   # never use :latest
          ports:
            - containerPort: 8080
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: myservice-secrets
                  key: database_url
          resources:
            requests:
              cpu: "100m"
              memory: "64Mi"
            limits:
              cpu: "500m"
              memory: "256Mi"
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /livez
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 20
          securityContext:
            runAsNonRoot: true
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
```

## Service and HorizontalPodAutoscaler

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: myservice
spec:
  selector:
    app: myservice
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: myservice
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: myservice
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

## PodDisruptionBudget

Prevents all pods from being evicted simultaneously during node maintenance:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: myservice-pdb
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: myservice
```

## Useful kubectl commands

```bash
# Watch rollout status
kubectl rollout status deployment/myservice

# Rollback to previous version
kubectl rollout undo deployment/myservice

# Get resource usage
kubectl top pods -l app=myservice

# Stream logs from all pods
kubectl logs -l app=myservice -f --max-log-requests=10

# Port-forward for debugging
kubectl port-forward deployment/myservice 8080:8080
```
