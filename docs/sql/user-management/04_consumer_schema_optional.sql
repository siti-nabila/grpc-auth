-- OPSIONAL. Jalankan di database consumer (misalnya CLM), bukan otomatis di auth.
-- Tidak ada seed; consumer mengisi record setelah efek bisnis committed.
BEGIN;
CREATE TABLE public.processed_events (
    consumer_name varchar(100) NOT NULL,
    event_id uuid NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer_name, event_id)
);
COMMIT;
