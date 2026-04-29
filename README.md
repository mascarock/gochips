# gochips

A high-performance, thread-safe Go HTTP API for transferring virtual chips between players with strict business rules, idempotency guarantees, and comprehensive test coverage.

## Features

- **Transfer Endpoint**: `POST /transfer-chips`
- **Balance Endpoint**: `GET /chip-balance/{playerId}`
- **Business Rules**:
  - Initial balance: 10,000 chips per player
  - Max transfer: 5,000 chips
  - No self-transfers
  - Positive amounts only
  - Idempotent transfers (duplicate transferId returns 409)
  - Proper error handling and HTTP status codes
- **Concurrency Safe**: Uses RWMutex with careful locking patterns
- **Comprehensive Testing**: Unit, handler, concurrency, and E2E tests

## API

### Transfer Chips
```http
POST /transfer-chips
Content-Type: application/json

{
  "transferId": "unique-tx-123",
  "fromPlayerId": "player1",
  "toPlayerId": "player2",
  "amount": 1000
}
```

### Get Balance
```http
GET /chip-balance/player1
```

## Running

```bash
go run .
# or
go build -o gochips
./gochips
```

Server listens on :3000 (or $PORT).

## Testing

```bash
go test -v
```

All tests pass including heavy concurrency scenarios (hundreds of goroutines, ring transfers, star topology, etc.).

---

*Last edited: ~10:50 AM CEST on April 29, 2026*
