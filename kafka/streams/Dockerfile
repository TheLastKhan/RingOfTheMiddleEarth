FROM maven:3.9-eclipse-temurin-17 AS builder

WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline -B

COPY src ./src
RUN mvn package -DskipTests -B

FROM eclipse-temurin:17-jre-alpine

RUN apk --no-cache add libstdc++

COPY --from=builder /app/target/kafka-streams-1.0.0.jar /app/kafka-streams.jar

ENTRYPOINT ["java", "-jar", "/app/kafka-streams.jar"]
