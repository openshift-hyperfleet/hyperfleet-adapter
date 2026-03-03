# Observability

## Broker Metrics

The adapter automatically registers Prometheus metrics from the broker library on the `/metrics` endpoint (port 9090). No additional configuration is needed.

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `hyperfleet_broker_messages_consumed_total` | Counter | Total messages consumed from the broker |
| `hyperfleet_broker_errors_total` | Counter | Total message processing errors (labels: `topic`, `error_type`) |
| `hyperfleet_broker_message_duration_seconds` | Histogram | Message processing duration |

These metrics use the `hyperfleet_broker_` prefix and include the adapter's `component` and `version` labels.
