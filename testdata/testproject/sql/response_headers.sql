create function http_response_headers(
  out status smallint,
  out response_headers jsonb
)
language plpgsql as $$
begin
  status := 200;
  response_headers := jsonb_build_object('foo', 'bar');
end;
$$;
