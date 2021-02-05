create function http_status_200_when_missing(
  out resp_body jsonb
)
language plpgsql as $$
begin
  resp_body := jsonb_build_object('foo', 'bar');
end;
$$;

create function http_status_200_when_null(
  out status smallint,
  out resp_body jsonb
)
language plpgsql as $$
begin
  resp_body := jsonb_build_object('foo', 'bar');
end;
$$;
