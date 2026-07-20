-- Revert 000005: remove the 'rejected' pickup order status.
--
-- PostgreSQL cannot drop a value from an enum, so the type is rebuilt without
-- 'rejected'. Any existing rejected orders are folded back into 'cancelled'
-- before the value disappears.
UPDATE public.pickup_orders SET status = 'cancelled' WHERE status = 'rejected';

ALTER TABLE public.pickup_orders ALTER COLUMN status DROP DEFAULT;

ALTER TYPE pickup_order_status RENAME TO pickup_order_status_old;

CREATE TYPE pickup_order_status AS ENUM ('pending', 'confirmed', 'cancelled', 'cancel_request');

ALTER TABLE public.pickup_orders
  ALTER COLUMN status TYPE pickup_order_status
  USING status::text::pickup_order_status;

ALTER TABLE public.pickup_orders ALTER COLUMN status SET DEFAULT 'pending';

DROP TYPE pickup_order_status_old;
