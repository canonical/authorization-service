CREATE DATABASE authorization-service;
CREATE DATABASE sts;
GRANT ALL PRIVILEGES ON DATABASE authorization-service TO authorization-service;
GRANT ALL PRIVILEGES ON DATABASE sts TO authorization-service;

ALTER DATABASE authorization-service OWNER TO authorization-service;
ALTER DATABASE sts OWNER TO authorization-service;
