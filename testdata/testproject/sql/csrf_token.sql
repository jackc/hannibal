create function http_get_csrf_token(
  out template text
)
language plpgsql as $$
begin
  template := 'csrf_token.html';
end;
$$;
