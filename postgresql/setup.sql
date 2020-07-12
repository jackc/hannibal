select hannibal.create_handler(
  'GET',
  '/hey',
  'select json_build_object(''time'', now(), ''name'', $1::text)',
  array[
    hannibal.build_handler_param('name', 'text', true)
  ]
);
