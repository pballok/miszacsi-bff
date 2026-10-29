-- name: GetServiceType :one
SELECT * FROM service_types WHERE code = $1;
