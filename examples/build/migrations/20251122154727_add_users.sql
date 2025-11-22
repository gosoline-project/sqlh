-- +goose Up
-- +goose StatementBegin
create table users
(
    id         int auto_increment primary key,
    name       varchar(255) not null,
    updated_at timestamp null,
    created_at timestamp null
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table users
-- +goose StatementEnd
