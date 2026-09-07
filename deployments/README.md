# Chat Deployment

## Preconditions

- Ensure deployed IM Server (xue-hua-server) and its dependencies.
  - Redis
  - MongoDB
  - Kafka
  - MinIO
- Expose the corresponding Services and ports of IM Server (xue-hua-server).

## Deploy Chat

**Chat depends on IM Server (xue-hua-server), so you need to deploy IM Server (xue-hua-server) first.**

enter the target directory

```shell
cd deployments/deploy
```

### Modify ConfigMap

You need to modify the `chat-config.yml` file to match your environment. Focus on the following fields:
**discovery.yml**

- `kubernetes.namespace`: default is `default`, you can change it to your namespace.

**mongodb.yml**

- `address`: set to your already mongodb address or mongo Service name and port in your deployed.
- `database`: set to your mongodb database name.(Need have a created database.)
- `authSource`: et to your mongodb authSource. (authSource is specify the database name associated with the user's credentials, user need create in this database.)

**redis.yml**

- `address`: set to your already redis address or redis Service name and port in your deployed.

**share.yml**

- `server.apiURL`: modify to your already API address or use your `server-api` service name and port
- `server.secret`: same to IM Server `share.secret` value.

### Set the secret

A Secret is an object that contains a small amount of sensitive data. Such as password and secret. Secret is similar to ConfigMaps.

#### Redis:

Update the `redis-password` value in `redis-secret.yml` to your Redis password encoded in base64.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: chat-redis-secret
type: Opaque
data:
  redis-password: b3BlbklNMTIz # update to your redis password encoded in base64, if need empty, you can set to ""
```

#### Mongo:

Update the `mongo_chat_username`, `mongo_chat_password` value in `mongo-secret.yml` to your Mongo username and password encoded in base64.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: chat-mongo-secret
type: Opaque
data:
  mongo_chat_username: b3BlbklN # update to your mongo username encoded in base64, if need empty, you can set to "" (this user credentials need in authSource database)
  mongo_chat_password: b3BlbklNMTIz # update to your mongo password encoded in base64, if need empty, you can set to ""
```

### Apply the secret.

```shell
kubectl apply -f redis-secret.yml -f mongo-secret.yml
```

### Apply Config and Services

deploy the config and services

```shell
kubectl apply -f chat-config.yml -f chat-api-admin-service.yml -f chat-api-chat-service.yml -f chat-rpc-admin-service.yml -f chat-rpc-chat-service.yml
```

### Start Chat Deployments

```shell
kubectl apply -f chat-api-chat-deployment.yml -f chat-api-admin-deployment.yml -f chat-rpc-chat-deployment.yml -f chat-rpc-admin-deployment.yml
```

## Verify

After the deployment is complete, you can verify the deployment status.

```shell
# Check the status of all pods
kubectl get pods

# Check the status of services
kubectl get svc

# Check the status of deployments
kubectl get deployments

# View all resources
kubectl get all

```

## clean all

`kubectl delete -f ./`

## Notes:

- If you use a specific namespace for your deployment, be sure to append the -n <namespace> flag to your kubectl commands.
