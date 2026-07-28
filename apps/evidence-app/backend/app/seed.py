"""
Run with: python -m app.seed
Populates a default Product, frameworks (SOC2, PCI-DSS, HIPAA), and their controls.
"""

from app.database import SessionLocal
from app.models.product import Product
from app.models.framework import Framework
from app.models.control import Control
import app.models.evidence  # noqa: F401 — needed to resolve ORM relationships
import app.models.submission  # noqa: F401

DEFAULT_PRODUCT_NAME = "WSO2 Cloud"
DEFAULT_PRODUCT_DESCRIPTION = (
    "Default product seeded for compliance evidence collection. "
    "Add more products via POST /api/products/."
)

SEED_DATA = [
    {
        "name": "SOC2",
        "description": "AICPA Trust Services Criteria for security, availability, processing integrity, confidentiality, and privacy.",
        "controls": [
            ("CC6.1", "Logical & Physical Access Controls", "The entity implements logical access security measures to protect against threats from sources outside its system boundaries."),
            ("CC6.2", "Credential Issuance & Removal", "Prior to issuing system credentials and granting system access, the entity registers and authorizes new internal and external users."),
            ("CC6.3", "Authentication Controls", "The entity authorizes, modifies, or removes access to data, software, functions, and other protected information assets based on approved and documented access rules."),
            ("CC6.6", "External Threat Controls", "The entity implements controls to prevent or detect and act upon the introduction of unauthorized or malicious software."),
            ("CC6.7", "Data Transmission Controls", "The entity restricts the transmission, movement, and removal of information to authorized internal and external users and processes."),
            ("CC7.1", "System Monitoring Configuration", "To meet its objectives, the entity uses detection and monitoring procedures to identify changes to configurations that result in the introduction of new vulnerabilities."),
            ("CC7.2", "Security Event Monitoring", "The entity monitors system components and the operation of those components for anomalies that are indicative of malicious acts, natural disasters, and errors affecting the entity's ability to meet its objectives."),
            ("CC7.4", "Incident Response", "The entity responds to identified security incidents by executing a defined incident response program to understand, contain, remediate, and communicate security incidents."),
            ("CC8.1", "Change Management", "The entity authorizes, designs, develops or acquires, configures, documents, tests, approves, and implements changes to infrastructure, data, software, and procedures."),
            ("CC9.1", "Risk Mitigation", "The entity identifies, selects, and develops risk mitigation activities for risks arising from potential business disruptions."),
            ("A1.1", "Availability Capacity Planning", "The entity maintains, monitors, and evaluates current processing capacity and use of system components to manage capacity demand."),
            ("PI1.1", "Processing Integrity Policies", "The entity obtains or generates, uses, and communicates relevant, quality information regarding the objectives related to processing."),
        ],
    },
    {
        "name": "PCI-DSS",
        "description": "Payment Card Industry Data Security Standard for organizations that handle cardholder data.",
        "controls": [
            ("Req 1.1", "Firewall Configuration", "Establish and implement firewall and router configuration standards."),
            ("Req 1.2", "Network Access Restrictions", "Restrict inbound and outbound traffic to that which is necessary for the cardholder data environment."),
            ("Req 2.1", "Default Credentials", "Do not use vendor-supplied defaults for system passwords and other security parameters."),
            ("Req 3.1", "Cardholder Data Retention", "Keep cardholder data storage to a minimum by implementing data retention and disposal policies."),
            ("Req 3.4", "PAN Rendering", "Render PAN unreadable anywhere it is stored by using strong cryptography."),
            ("Req 6.1", "Vulnerability Management", "Establish a process to identify security vulnerabilities using reputable outside sources."),
            ("Req 6.2", "Security Patches", "Protect all system components and software from known vulnerabilities by installing applicable security patches."),
            ("Req 7.1", "Access Restriction", "Limit access to system components and cardholder data to only those individuals whose job requires such access."),
            ("Req 8.2", "User Identification", "Proper identification and authentication management for non-consumer users and administrators."),
            ("Req 8.3", "Multi-Factor Authentication", "Secure individual non-consumer authentication and all administrator access into the cardholder data environment using multi-factor authentication."),
            ("Req 10.1", "Audit Logs", "Implement audit trails to link all access to system components to each individual user."),
            ("Req 10.2", "Automated Audit Trails", "Implement automated audit trails for all system components to reconstruct events."),
            ("Req 11.2", "Vulnerability Scans", "Run internal and external network vulnerability scans at least quarterly."),
            ("Req 12.1", "Security Policy", "Establish, publish, maintain, and disseminate a security policy."),
        ],
    },
    {
        "name": "HIPAA",
        "description": "Health Insurance Portability and Accountability Act — Security Rule for protecting electronic Protected Health Information (ePHI).",
        "controls": [
            ("§164.308(a)(1)", "Risk Analysis", "Conduct an accurate and thorough assessment of the potential risks and vulnerabilities to the confidentiality, integrity, and availability of ePHI."),
            ("§164.308(a)(3)", "Workforce Authorization", "Implement procedures to authorize and/or supervise workforce members who work with ePHI."),
            ("§164.308(a)(4)", "Information Access Management", "Implement policies and procedures for authorizing access to ePHI that are consistent with applicable requirements."),
            ("§164.308(a)(5)", "Security Awareness Training", "Implement a security awareness and training program for all workforce members."),
            ("§164.308(a)(6)", "Security Incident Procedures", "Implement policies and procedures to address security incidents."),
            ("§164.310(a)(1)", "Facility Access Controls", "Implement policies and procedures to limit physical access to its electronic information systems and the facility in which they are housed."),
            ("§164.310(b)", "Workstation Use", "Implement policies and procedures that specify the proper functions to be performed, the manner in which those functions are to be performed, and the physical attributes of the surroundings of a specific workstation."),
            ("§164.312(a)(1)", "Access Control", "Implement technical policies and procedures for electronic information systems that maintain ePHI to allow access only to authorized persons or software programs."),
            ("§164.312(b)", "Audit Controls", "Implement hardware, software, and/or procedural mechanisms that record and examine activity in information systems that contain or use ePHI."),
            ("§164.312(c)(1)", "Integrity Controls", "Implement policies and procedures to protect ePHI from improper alteration or destruction."),
            ("§164.312(d)", "Authentication", "Implement procedures to verify that a person or entity seeking access to ePHI is the one claimed."),
            ("§164.312(e)(1)", "Transmission Security", "Implement technical security measures to guard against unauthorized access to ePHI that is being transmitted over an electronic communications network."),
        ],
    },
]


def seed():
    db = SessionLocal()
    try:
        if db.query(Framework).count() > 0:
            print("Database already seeded — skipping.")
            return

        # Ensure default Product exists (frameworks now require product_id).
        product = db.query(Product).filter(Product.name == DEFAULT_PRODUCT_NAME).first()
        if not product:
            product = Product(
                name=DEFAULT_PRODUCT_NAME,
                description=DEFAULT_PRODUCT_DESCRIPTION,
            )
            db.add(product)
            db.flush()
            print(f"  Created default product: {DEFAULT_PRODUCT_NAME} (id={product.id})")

        for fw_data in SEED_DATA:
            framework = Framework(
                product_id=product.id,
                name=fw_data["name"],
                description=fw_data["description"],
            )
            db.add(framework)
            db.flush()

            for ref, title, description in fw_data["controls"]:
                control = Control(
                    framework_id=framework.id,
                    control_ref=ref,
                    title=title,
                    description=description,
                )
                db.add(control)

            print(f"  {fw_data['name']}: {len(fw_data['controls'])} controls added")

        db.commit()
        print("Seeding complete.")
    except Exception as e:
        db.rollback()
        raise e
    finally:
        db.close()


if __name__ == "__main__":
    seed()
