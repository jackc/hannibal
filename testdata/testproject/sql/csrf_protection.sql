create function http_get_csrf_token(
  out template text
)
language plpgsql as $$
begin
  template := 'csrf_token.html';
end;
$$;

create function http_handle_csrf_failure(
  out status smallint,
  out template text
)
language plpgsql as $$
begin
  status := 403;
  template := 'csrf_failure.html';
end;
$$;
