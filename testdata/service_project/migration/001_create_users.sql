create table users (
  id int primary key generated by default as identity,
  username text not null,
  password_digest text not null
);