# gsm-gateway

A process which acts as an sms gateway using a gsm modem.  The gateway uses postgres to store message information, and is configured to send and receive messages.

## Receiving messages

When receiving a message it will post the message in json format to an http endpoint configured with the `NOTIFICATION_URL` environment variable.

```
{
    "number": "15555555555",
    "body": "hi",
    "time": "2018-04-28T20:56:07.852231807Z"
}
```

## Sending messages

To send a message, post a similar format to the `/api/messages` endpoint.

```
curl -X POST http://localhost:8080/api/messages -d '{"number":"17783175526","body":"hello"}'
```

The intent is that the gateway should continue running and log errors.  Proper testing of stability has not been done yet.