package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aurora-vm/aurora/internal/app/account"
	"github.com/aurora-vm/aurora/internal/app/apikeys"
	appAudit "github.com/aurora-vm/aurora/internal/app/audit"
	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/app/authz"
	appBilling "github.com/aurora-vm/aurora/internal/app/billing"
	appCompute "github.com/aurora-vm/aurora/internal/app/compute"
	appConsole "github.com/aurora-vm/aurora/internal/app/console"
	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	appDiagnostics "github.com/aurora-vm/aurora/internal/app/diagnostics"
	appEvents "github.com/aurora-vm/aurora/internal/app/events"
	appEvacuation "github.com/aurora-vm/aurora/internal/app/evacuation"
	appHealth "github.com/aurora-vm/aurora/internal/app/health"
	appIPAM "github.com/aurora-vm/aurora/internal/app/ipam"
	appJob "github.com/aurora-vm/aurora/internal/app/job"
	appKeyRotation "github.com/aurora-vm/aurora/internal/app/keyrotation"
	appMigration "github.com/aurora-vm/aurora/internal/app/migration"
	appMonitoring "github.com/aurora-vm/aurora/internal/app/monitoring"
	appNetwork "github.com/aurora-vm/aurora/internal/app/network"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	appNodeHealth "github.com/aurora-vm/aurora/internal/app/nodehealth"
	appNotification "github.com/aurora-vm/aurora/internal/app/notification"
	appRateLimit "github.com/aurora-vm/aurora/internal/app/ratelimit"
	appReconcile "github.com/aurora-vm/aurora/internal/app/reconcile"
	appRecovery "github.com/aurora-vm/aurora/internal/app/recovery"
	appScheduler "github.com/aurora-vm/aurora/internal/app/scheduler"
	appStorage "github.com/aurora-vm/aurora/internal/app/storage"
	appTemplate "github.com/aurora-vm/aurora/internal/app/template"
	appWebhook "github.com/aurora-vm/aurora/internal/app/webhook"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainBilling "github.com/aurora-vm/aurora/internal/domain/billing"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainIPAM "github.com/aurora-vm/aurora/internal/domain/ipam"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	domainMonitoring "github.com/aurora-vm/aurora/internal/domain/monitoring"
	domainNetwork "github.com/aurora-vm/aurora/internal/domain/network"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainNotification "github.com/aurora-vm/aurora/internal/domain/notification"
	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
	domainReconcile "github.com/aurora-vm/aurora/internal/domain/reconcile"
	domainStorage "github.com/aurora-vm/aurora/internal/domain/storage"
	domainTemplate "github.com/aurora-vm/aurora/internal/domain/template"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/aurora-vm/aurora/internal/infra/config"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/email"
	"github.com/aurora-vm/aurora/internal/infra/imagesource"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/aurora-vm/aurora/internal/infra/postgres"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/siem"
	infraStorage "github.com/aurora-vm/aurora/internal/infra/storage"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/aurora-vm/aurora/internal/infra/webhooks"
	transportGRPC "github.com/aurora-vm/aurora/internal/transport/grpc"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
	transportWS "github.com/aurora-vm/aurora/internal/transport/ws"
	"github.com/aurora-vm/aurora/pkg/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	vInfo := version.Get()
	log.Printf("Starting Aurora Control Plane Server [%s]...", vInfo.String())

	cfg := config.LoadServerConfig()
	log.Printf("Boot configuration: %s", cfg.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize Cryptographic, PKI & Security Infrastructure
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector(cfg.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize secret protector: %v", err)
	}

	tokenManager, err := jwt.NewTokenManager(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("Failed to initialize token manager: %v", err)
	}

	totpManager := totp.NewTOTPManager()

	// Initialize or load persistent Internal Root CA
	tlsDir := os.Getenv("AURORA_TLS_DIR")
	if tlsDir == "" {
		tlsDir = os.Getenv("AURORA_STATE_DIR")
		if tlsDir != "" {
			tlsDir = filepath.Join(tlsDir, "tls")
		} else {
			tlsDir = "/var/lib/aurora/tls"
		}
	}

	var internalCA *pki.InternalCA
	caCertPath := filepath.Join(tlsDir, "ca.crt")
	caKeyPath := filepath.Join(tlsDir, "ca.key")

	if certBytes, err := os.ReadFile(caCertPath); err == nil {
		if keyBytes, err := os.ReadFile(caKeyPath); err == nil {
			if loadedCA, err := pki.NewInternalCA(certBytes, keyBytes); err == nil {
				internalCA = loadedCA
				log.Printf("[INFO] Loaded existing Internal Root CA from %s", caCertPath)
			}
		}
	}

	if internalCA == nil {
		newCA, err := pki.NewInternalCA(nil, nil)
		if err != nil {
			log.Fatalf("Failed to initialize Internal CA: %v", err)
		}
		internalCA = newCA
		if err := os.MkdirAll(tlsDir, 0700); err == nil {
			_ = os.WriteFile(caCertPath, internalCA.GetCACertificatePEM(), 0644)
			_ = os.WriteFile(caKeyPath, internalCA.GetCAKeyPEM(), 0600)
			log.Printf("[INFO] Initialized and persisted new Internal Root CA at %s", tlsDir)
		}
	}

	// 2. Initialize Persistence Layer
	var userRepo identity.UserRepository
	var roleRepo identity.RoleRepository
	var apiKeyRepo identity.APIKeyRepository
	var sessionRepo identity.SessionRepository
	var auditRepo domainAudit.Repository
	var siemRepo domainAudit.SIEMRepository
	var nodeRepo domainNode.NodeRepository
	var enrollRepo domainNode.EnrollmentRepository
	var instRepo domainCompute.InstanceRepository
	var ipPoolRepo domainIPAM.IPPoolRepository
	var ipAllocRepo domainIPAM.IPAllocationRepository
	var firewallRepo domainNetwork.FirewallRepository
	var storagePoolRepo domainStorage.StoragePoolRepository
	var volumeRepo domainStorage.VolumeRepository
	var snapshotRepo domainStorage.VolumeSnapshotRepository
	var metricsRepo domainMonitoring.MetricsRepository
	var thresholdRepo domainMonitoring.AlertThresholdRepository
	var alertEventRepo domainMonitoring.AlertEventRepository
	var templateRepo domainTemplate.TemplateRepository
	var imageRepo domainTemplate.ImageRepository
	var planRepo domainBilling.PlanRepository
	var subRepo domainBilling.SubscriptionRepository
	var quotaRepo domainBilling.QuotaRepository
	var usageRepo domainBilling.UsageRepository
	var invoiceRepo domainBilling.InvoiceRepository
	var eventRepo domainEvents.Repository
	var notifRepo domainNotification.NotificationRepository
	var prefRepo domainNotification.PreferenceRepository
	var webhookRepo domainWebhook.WebhookRepository
	var deliveryRepo domainWebhook.DeliveryRepository
	var jobRepo domainJob.JobRepository
	var leaseRepo domainJob.WorkerLeaseRepository
	var migrationRepo domainMigration.MigrationRepository
	var rateLimiter domainRateLimit.Limiter
	var backupRepo domainBackup.Repository
	var reconcileRepo domainReconcile.Repository
	var keyRotationRepo domainKeyRotation.Repository

	var db *postgres.DB
	if cfg.DatabaseURL != "" {
		dbCtx, dbCancel := context.WithTimeout(ctx, 3*time.Second)
		db, err = postgres.NewPool(dbCtx, cfg.DatabaseURL)
		dbCancel()
		if err != nil {
			log.Printf("[WARN] PostgreSQL unavailable at boot: %v. Running in-memory store for fallback.", err)
			memStore := memory.NewMemoryStore()
			userRepo = memStore.Users()
			roleRepo = memStore.Roles()
			apiKeyRepo = memStore.APIKeys()
			sessionRepo = memStore.Sessions()
			auditRepo = memStore.Audit()
			siemRepo = memStore.SIEM()
			nodeRepo = memStore.Nodes()
			enrollRepo = memStore.Enrollments()
			instRepo = memStore.Instances()
			ipPoolRepo = memStore.IPPools()
			ipAllocRepo = memStore.IPAllocations()
			firewallRepo = memStore.Firewall()
			storagePoolRepo = memStore.StoragePools()
			volumeRepo = memStore.Volumes()
			snapshotRepo = memStore.Snapshots()
			metricsRepo = memStore.Metrics()
			thresholdRepo = memStore.AlertThresholds()
			alertEventRepo = memStore.AlertEvents()
			templateRepo = memStore.Templates()
			imageRepo = memStore.Images()
			planRepo = memStore.Plans()
			subRepo = memStore.Subscriptions()
			quotaRepo = memStore.Quotas()
			usageRepo = memStore.Usage()
			invoiceRepo = memStore.Invoices()
			eventRepo = memStore.Events()
			notifRepo = memStore.Notifications()
			prefRepo = memStore.Preferences()
			webhookRepo = memStore.Webhooks()
			deliveryRepo = memStore.Deliveries()
			jobRepo = memStore.Jobs()
			leaseRepo = memStore.Leases()
			migrationRepo = memStore.Migrations()
			rateLimiter = memStore.RateLimiter()
			backupRepo = memStore.Backups()
			reconcileRepo = memStore.Reconcile()
			keyRotationRepo = memStore.KeyRotations()
		} else {
			log.Println("[INFO] PostgreSQL connection pool initialized successfully.")
			defer db.Close()

			if cfg.AutoMigrate {
				migrator := postgres.NewMigrator(db.Pool, cfg.MigrationsDir)
				applied, err := migrator.Up(ctx)
				if err != nil {
					log.Printf("[ERROR] Database migration failed: %v", err)
				} else {
					log.Printf("[INFO] Database migrations verified. %d applied.", applied)
				}
			}

			userRepo = postgres.NewUserRepository(db.Pool)
			roleRepo = postgres.NewRoleRepository(db.Pool)
			apiKeyRepo = postgres.NewAPIKeyRepository(db.Pool)
			sessionRepo = postgres.NewSessionRepository(db.Pool)
			auditRepo = postgres.NewAuditRepository(db.Pool)
			siemRepo = postgres.NewSIEMRepository(db.Pool)
			nodeRepo = postgres.NewNodeRepository(db.Pool)
			enrollRepo = postgres.NewEnrollmentRepository(db.Pool)
			instRepo = postgres.NewInstanceRepository(db.Pool)
			ipPoolRepo = postgres.NewIPPoolRepository(db.Pool)
			ipAllocRepo = postgres.NewIPAllocationRepository(db.Pool)
			firewallRepo = postgres.NewFirewallRepository(db.Pool)
			storagePoolRepo = postgres.NewStoragePoolRepository(db.Pool)
			volumeRepo = postgres.NewVolumeRepository(db.Pool)
			snapshotRepo = postgres.NewVolumeSnapshotRepository(db.Pool)
			metricsRepo = postgres.NewMetricsRepository(db.Pool)
			thresholdRepo = postgres.NewAlertThresholdRepository(db.Pool)
			alertEventRepo = postgres.NewAlertEventRepository(db.Pool)
			templateRepo = postgres.NewTemplateRepository(db.Pool)
			imageRepo = postgres.NewImageRepository(db.Pool)
			planRepo = postgres.NewPlanRepository(db.Pool)
			subRepo = postgres.NewSubscriptionRepository(db.Pool)
			quotaRepo = postgres.NewQuotaRepository(db.Pool)
			usageRepo = postgres.NewUsageRepository(db.Pool)
			invoiceRepo = postgres.NewInvoiceRepository(db.Pool)
			eventRepo = postgres.NewEventRepository(db.Pool)
			notifRepo = postgres.NewNotificationRepository(db.Pool)
			prefRepo = postgres.NewPreferenceRepository(db.Pool)
			webhookRepo = postgres.NewWebhookRepository(db.Pool)
			deliveryRepo = postgres.NewDeliveryRepository(db.Pool)
			jobRepo = postgres.NewJobRepository(db.Pool)
			leaseRepo = postgres.NewWorkerLeaseRepository(db.Pool)
			migrationRepo = postgres.NewMigrationRepository(db.Pool)
			rateLimiter = postgres.NewRateLimiter(db.Pool)
			backupRepo = postgres.NewBackupRepository(db.Pool)
			reconcileRepo = postgres.NewReconcileRepository(db.Pool)
			keyRotationRepo = postgres.NewKeyRotationRepository(db.Pool)
		}
	} else {
		memStore := memory.NewMemoryStore()
		userRepo = memStore.Users()
		roleRepo = memStore.Roles()
		apiKeyRepo = memStore.APIKeys()
		sessionRepo = memStore.Sessions()
		auditRepo = memStore.Audit()
		siemRepo = memStore.SIEM()
		nodeRepo = memStore.Nodes()
		enrollRepo = memStore.Enrollments()
		instRepo = memStore.Instances()
		ipPoolRepo = memStore.IPPools()
		ipAllocRepo = memStore.IPAllocations()
		firewallRepo = memStore.Firewall()
		storagePoolRepo = memStore.StoragePools()
		volumeRepo = memStore.Volumes()
		snapshotRepo = memStore.Snapshots()
		metricsRepo = memStore.Metrics()
		thresholdRepo = memStore.AlertThresholds()
		alertEventRepo = memStore.AlertEvents()
		templateRepo = memStore.Templates()
		imageRepo = memStore.Images()
		planRepo = memStore.Plans()
		subRepo = memStore.Subscriptions()
		quotaRepo = memStore.Quotas()
		usageRepo = memStore.Usage()
		invoiceRepo = memStore.Invoices()
		eventRepo = memStore.Events()
		notifRepo = memStore.Notifications()
		prefRepo = memStore.Preferences()
		webhookRepo = memStore.Webhooks()
		deliveryRepo = memStore.Deliveries()
		jobRepo = memStore.Jobs()
		leaseRepo = memStore.Leases()
		migrationRepo = memStore.Migrations()
		rateLimiter = memStore.RateLimiter()
		backupRepo = memStore.Backups()
		reconcileRepo = memStore.Reconcile()
		keyRotationRepo = memStore.KeyRotations()
	}

	// 3. Initialize Application Services, Event Bus & Dispatchers
	authorizer := authz.NewAuthorizer(roleRepo)
	siemDispatcher := siem.NewDispatcher(siemRepo, 2000)
	defer siemDispatcher.Close()

	eventBus := appEvents.NewEventBus(eventRepo, 2048, 8)
	defer eventBus.Close()

	emailProvider := email.NewSimulatedEmailProvider()
	webhookDispatcher := webhooks.NewDispatcher(webhookRepo, deliveryRepo)
	defer webhookDispatcher.Close()

	auditService := appAudit.NewService(auditRepo, siemRepo, siemDispatcher, authorizer)
	authService := auth.NewService(userRepo, roleRepo, sessionRepo, hasher, protector, tokenManager, totpManager, auditService)
	acctService := account.NewService(userRepo, hasher, protector, totpManager, auditService)
	apiKeyService := apikeys.NewService(apiKeyRepo, userRepo, roleRepo, auditService)
	healthService := appHealth.NewService()

	connManager := appNode.NewConnectionManager()
	hubEndpoint := fmt.Sprintf("127.0.0.1:%d", cfg.GRPCPort)
	nodeService := appNode.NewService(nodeRepo, enrollRepo, internalCA, connManager, auditService, hubEndpoint)

	schedulerService := appScheduler.NewScheduler(nodeRepo, instRepo)

	computeService := appCompute.NewService(instRepo, nodeRepo, nodeService, authorizer, auditService)
	computeService.SetEventPublisher(eventBus)
	computeService.SetScheduler(schedulerService)

	imageSourceRegistry := imagesource.NewRegistry([]string{"images", "ubuntu", "local"})
	templateService := appTemplate.NewService(templateRepo, imageRepo, nodeRepo, nodeService, imageSourceRegistry, authorizer, auditService)
	computeService.SetTemplateService(templateService)

	paymentProvider := appBilling.NewSimulatedPaymentProvider()
	billingService := appBilling.NewService(planRepo, subRepo, quotaRepo, usageRepo, invoiceRepo, paymentProvider, authorizer, auditService)
	computeService.SetQuotaValidator(billingService.QuotaEngine())

	ipamService := appIPAM.NewService(ipPoolRepo, ipAllocRepo, authorizer, auditService)
	networkService := appNetwork.NewService(firewallRepo, instRepo, nodeRepo, nodeService, authorizer, auditService)
	storageService := appStorage.NewService(storagePoolRepo, volumeRepo, snapshotRepo, instRepo, nodeRepo, nodeService, authorizer, auditService)
	monitoringService := appMonitoring.NewService(metricsRepo, thresholdRepo, alertEventRepo, instRepo, nodeRepo, authorizer, auditService)

	notificationService := appNotification.NewService(notifRepo, prefRepo, emailProvider, authorizer, auditService)
	webhookService := appWebhook.NewService(webhookRepo, deliveryRepo, webhookDispatcher, authorizer, auditService)

	// Phase 15: Job Orchestration, Migrations, Evacuation & Self-Healing
	jobEngine := appJob.NewEngine(jobRepo, leaseRepo, authorizer, auditService, 4)
	defer jobEngine.Close()
	jobEngine.SetEventPublisher(eventBus)

	migrationService := appMigration.NewService(migrationRepo, instRepo, nodeRepo, nodeService, schedulerService, authorizer, auditService)
	migrationService.SetEventPublisher(eventBus)

	evacuationService := appEvacuation.NewService(nodeRepo, instRepo, migrationService, authorizer, auditService)
	evacuationService.SetEventPublisher(eventBus)

	healthSupervisor := appNodeHealth.NewSupervisor(nodeRepo, auditService, 5*time.Second)
	defer healthSupervisor.Close()
	healthSupervisor.SetEventPublisher(eventBus)

	rateLimitService := appRateLimit.NewService(rateLimiter)
	metricsCollector := transportHTTP.NewMetricsCollector(jobRepo, nodeRepo, instRepo)

	// Phase 16: Disaster Recovery, Backup Engine, Key Lifecycle & Diagnostics
	memObjStorage := infraStorage.NewMemoryObjectStorage()
	encStorage, _ := infraStorage.NewEncryptedStorageWrapper(memObjStorage, []byte(cfg.JWTSecret))

	backupService := appBackup.NewService(backupRepo, encStorage, authorizer, auditService, eventBus)
	reconcileService := appReconcile.NewService(reconcileRepo, instRepo, nodeRepo, jobRepo, migrationRepo, quotaRepo, authorizer, auditService)
	recoveryCoord := appRecovery.NewCoordinator(backupService, reconcileService, backupRepo, auditService, eventBus, authorizer)
	keyRotationService := appKeyRotation.NewService(keyRotationRepo, authorizer, auditService, eventBus)
	diagnosticsService := appDiagnostics.NewService(nodeRepo, instRepo, jobRepo, migrationRepo, storagePoolRepo, ipPoolRepo, auditRepo, backupRepo, quotaRepo, webhookRepo, authorizer)

	// Startup State Reconciliation & Safe Auto-Repair
	if recReport, err := reconcileService.Reconcile(ctx, false, "startup"); err == nil {
		log.Printf("[INFO] Startup state reconciliation completed: %d discrepancies found, %d auto-repairs made (took %dms)",
			recReport.TotalDiscrepancies, recReport.RepairedCount, recReport.DurationMs)
	}

	// Subscribe notification service and webhook dispatcher to event bus
	eventBus.Subscribe("*", notificationService.HandleEvent)
	eventBus.Subscribe("*", webhookDispatcher.DispatchEvent)

	// 4. Setup gRPC Server with mTLS PKI
	serverCertPEM, serverKeyPEM, err := internalCA.GenerateServerCertificate([]string{"127.0.0.1", "localhost"}, 365*24*time.Hour)
	if err != nil {
		log.Fatalf("Failed to generate gRPC server certificate: %v", err)
	}

	serverTLSConfig, err := internalCA.BuildServerTLSConfig(serverCertPEM, serverKeyPEM)
	if err != nil {
		log.Fatalf("Failed to build server mTLS config: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLSConfig)))
	gatewayServer := transportGRPC.Register(grpcServer, healthService, nodeService)

	// Setup Interactive Web Terminal & VNC Console Manager
	consoleManager := appConsole.NewManager(instRepo, nodeRepo, authorizer, auditService, gatewayServer.SendToNode)
	if gatewayServer != nil {
		gatewayServer.SetConsoleRouter(consoleManager.HandleNodeMessage)
	}

	// 5. Setup HTTP Router & Middlewares
	router := transportHTTP.NewRouter()
	router.Use(transportHTTP.SecurityHeadersMiddleware(10 * 1024 * 1024))

	authMiddleware := transportHTTP.AuthenticateMiddleware(tokenManager, apiKeyService)

	healthHandler := transportHTTP.NewHealthHandler(healthService)
	healthHandler.RegisterRoutes(router)

	metricsHandler := transportHTTP.NewMetricsHandler(metricsCollector)
	metricsHandler.RegisterRoutes(router)

	authHandler := transportHTTP.NewAuthHandler(authService)
	authHandler.RegisterRoutes(router, authMiddleware)

	accountHandler := transportHTTP.NewAccountHandler(acctService)
	accountHandler.RegisterRoutes(router, authMiddleware)

	apiKeyHandler := transportHTTP.NewAPIKeyHandler(apiKeyService, authorizer)
	apiKeyHandler.RegisterRoutes(router, authMiddleware)

	nodeHandler := transportHTTP.NewNodeHandler(nodeService, authorizer)
	nodeHandler.RegisterRoutes(router, authMiddleware)

	instanceHandler := transportHTTP.NewInstanceHandler(computeService, networkService, authorizer)
	instanceHandler.RegisterRoutes(router, authMiddleware)

	ipamHandler := transportHTTP.NewIPAMHandler(ipamService, authorizer)
	ipamHandler.RegisterRoutes(router, authMiddleware)

	storageHandler := transportHTTP.NewStorageHandler(storageService, authorizer)
	storageHandler.RegisterRoutes(router, authMiddleware)

	monitoringHandler := transportHTTP.NewMonitoringHandler(monitoringService, authorizer)
	monitoringHandler.RegisterRoutes(router, authMiddleware)

	auditHandler := transportHTTP.NewAuditHandler(auditService, authorizer)
	auditHandler.RegisterRoutes(router, authMiddleware)

	templateHandler := transportHTTP.NewTemplateHandler(templateService, authorizer)
	templateHandler.RegisterRoutes(router, authMiddleware)

	billingHandler := transportHTTP.NewBillingHandler(billingService, authorizer)
	billingHandler.RegisterRoutes(router, authMiddleware)

	notificationHandler := transportHTTP.NewNotificationHandler(notificationService, authorizer)
	notificationHandler.RegisterRoutes(router, authMiddleware)

	webhookHandler := transportHTTP.NewWebhookHandler(webhookService, authorizer)
	webhookHandler.RegisterRoutes(router, authMiddleware)

	eventHandler := transportHTTP.NewEventHandler(eventRepo, deliveryRepo, authorizer)
	eventHandler.RegisterRoutes(router, authMiddleware)

	jobHandler := transportHTTP.NewJobHandler(jobEngine, authorizer)
	jobHandler.RegisterRoutes(router, authMiddleware)

	migrationHandler := transportHTTP.NewMigrationHandler(migrationService, evacuationService, authorizer)
	migrationHandler.RegisterRoutes(router, authMiddleware)

	backupHandler := transportHTTP.NewBackupHandler(backupService, recoveryCoord, authorizer)
	backupHandler.RegisterRoutes(router, authMiddleware)

	reconcileHandler := transportHTTP.NewReconcileHandler(reconcileService, authorizer)
	reconcileHandler.RegisterRoutes(router, authMiddleware)

	keyRotationHandler := transportHTTP.NewKeyRotationHandler(keyRotationService, authorizer)
	keyRotationHandler.RegisterRoutes(router, authMiddleware)

	diagnosticsHandler := transportHTTP.NewDiagnosticsHandler(diagnosticsService, authorizer)
	diagnosticsHandler.RegisterRoutes(router, authMiddleware)

	consoleHandler := transportWS.NewConsoleHandler(consoleManager, tokenManager)
	consoleHandler.RegisterRoutes(router)

	eventStreamHandler := transportWS.NewEventStreamHandler(eventBus, tokenManager)
	router.Handle("/api/v1/events/stream", eventStreamHandler)

	_ = rateLimitService

	// 6. Mount Web SPA Frontend
	transportHTTP.RegisterSPARoutes(router, "web/dist", "./dist", "../web/dist")

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("Failed to listen for gRPC on port %d: %v", cfg.GRPCPort, err)
	}

	go func() {
		log.Printf("[INFO] HTTP API listening on http://0.0.0.0:%d", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	go func() {
		log.Printf("[INFO] gRPC mTLS Gateway listening on 0.0.0.0:%d", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM)
	<-shutdownSig

	log.Println("[INFO] Shutdown signal received. Performing graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server shutdown error: %v", err)
	}

	log.Println("[INFO] Aurora Control Plane stopped cleanly.")
}
