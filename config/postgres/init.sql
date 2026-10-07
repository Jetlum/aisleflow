-- Local demo credentials only. Runtime role cannot bypass row-level security.
CREATE ROLE aisleflow_app LOGIN PASSWORD 'aisleflow_demo' NOSUPERUSER NOBYPASSRLS;
GRANT CONNECT ON DATABASE aisleflow TO aisleflow_app;
GRANT USAGE ON SCHEMA public TO aisleflow_app;
