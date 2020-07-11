-- select create_handler(
--   'GET',
--   '/hey',
--   'select json_build_object(''time'', now(), ''name'', $1::text)',
--   array[
--     build_handler_param('name', 'text', true)
--   ]
-- );

create function sub(int, int) returns int
language sql as $$
  select $1 - $2;
$$;
