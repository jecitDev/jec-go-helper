# SERVICE API GATEWAY

This service provide authentication for application and API GATEWAY as service discovery for an other service.
## Features

- Authentication (Sign Up, Sign In and Sign Out)
    - Sign In
    Sign In will response JWT as Access Token And Refresh Token.
    This service use redis to store access with specfic duration. The duration of access storage will increase when the client accesses the API. When access deleted from redis, client will be unauthenticated.
    - Sign Out
    Sign out will delete access from redis. 
- API Gateway
API Gateway used as a service discovery. So that client can access the other service from API.
## ENVIRONTMENT VARIABLE

```env
POSTGRES_HOST=""
POSTGRES_PORT=""
POSTGRES_DBNAME=""
POSTGRES_USER=""
POSTGRES_PASSWORD=""
POSTGRES_SSLMODE="disable"

REDIS_HOST=""
REDIS_PORT=""
REDIS_PASSWORD=""

NEWRELIC_LICENSE=""
```
